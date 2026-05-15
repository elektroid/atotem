// ============================================================
//  TOTEM — client-side application
// ============================================================

const state = {
  phase: 'loading',  // loading | intro | quiz | tiebreaker | reveal | name | generating | complete
  questions: null,   // parsed questions.json
  animals: null,     // parsed animals.json
  currentIdx: 0,
  answers: {},       // { questionId: { scores, optionIdx } }
  tiebreakerQ: null, // tiebreaker_pair object
  totem: null,       // animal object
  shadow: null,      // animal object
  name: '',
  resultUUID: null,
  imageURL: null,
}

// ── Element encoding ──────────────────────────────────────────────────────────
// [terre, air, feu, eau]
const ELEM_VEC = {
  'terre':     [1,   0,   0,   0  ],
  'air':       [0,   1,   0,   0  ],
  'feu':       [0,   0,   1,   0  ],
  'eau':       [0,   0,   0,   1  ],
  'terre/feu': [0.5, 0,   0.5, 0  ],
  'eau/terre': [0.5, 0,   0,   0.5],
  'air/terre': [0.5, 0.5, 0,   0  ],
}

// ── Scoring ───────────────────────────────────────────────────────────────────

function userElementVector() {
  const qs = state.questions.core_questions.filter(q => q.group === 'element')
  const sum = [0, 0, 0, 0]
  let n = 0
  for (const q of qs) {
    const a = state.answers[q.id]
    if (a) {
      const opt = q.options[a.optionIdx]
      const vec = ELEM_VEC[opt.scores.element] || [0.25, 0.25, 0.25, 0.25]
      vec.forEach((v, i) => sum[i] += v)
      n++
    }
  }
  return n > 0 ? sum.map(v => v / n) : [0.25, 0.25, 0.25, 0.25]
}

function userFloatScore(dim) {
  const qs = state.questions.core_questions.filter(q => q.group === dim)
  const scored = qs.filter(q => state.answers[q.id] !== undefined)
  if (scored.length === 0) return 0.5
  const total = scored.reduce((s, q) => {
    const opt = q.options[state.answers[q.id].optionIdx]
    return s + (opt.scores[dim] ?? 0.5)
  }, 0)
  return total / scored.length
}

function distanceTo(animal) {
  const w = state.questions.meta.element_weight
  const uElem = userElementVector()
  const aElem = ELEM_VEC[animal.element] || [0.25, 0.25, 0.25, 0.25]
  const elemSq = aElem.reduce((s, v, i) => s + (uElem[i] - v) ** 2, 0)

  const dims = ['introvert_extrovert', 'intuition_sensing', 'thinking_feeling', 'structure_freedom']
  const persSq = dims.reduce((s, d) => {
    const av = animal.personality_profile?.[d] ?? 0.5
    return s + (userFloatScore(d) - av) ** 2
  }, 0)

  return Math.sqrt(w * w * elemSq + persSq)
}

function rankAnimals() {
  return state.animals.animals
    .map(a => ({ animal: a, dist: distanceTo(a) }))
    .sort((a, b) => a.dist - b.dist)
}

function findTiebreakerPair(id1, id2) {
  return state.questions.tiebreaker_pairs.find(p =>
    (p.animals[0] === id1 && p.animals[1] === id2) ||
    (p.animals[0] === id2 && p.animals[1] === id1)
  ) || null
}

function resolveResult() {
  const ranked = rankAnimals()
  const threshold = state.questions.meta.tiebreaker_threshold
  const top = ranked[0]
  const second = ranked[1]

  if (second && (second.dist - top.dist) < threshold) {
    const tb = findTiebreakerPair(top.animal.id, second.animal.id)
    if (tb) {
      state.tiebreakerQ = tb
      transition('tiebreaker')
      return
    }
  }
  applyTotem(top.animal)
}

function applyTotem(animal) {
  state.totem = animal
  state.shadow = state.animals.animals.find(a => a.id === animal.shadow_animal) || null
  transition('reveal')
}

// ── Actions ───────────────────────────────────────────────────────────────────

function transition(phase) {
  state.phase = phase
  render()
}

function answerCore(questionId, optionIdx) {
  state.answers[questionId] = { optionIdx }
  const qs = state.questions.core_questions
  if (state.currentIdx < qs.length - 1) {
    state.currentIdx++
    render()
  } else {
    resolveResult()
  }
}

function answerTiebreaker(optionIdx) {
  const winnerId = state.tiebreakerQ.options[optionIdx].points_to
  const winner = state.animals.animals.find(a => a.id === winnerId)
  applyTotem(winner)
}

async function submitName(name) {
  state.name = name.trim()
  transition('generating')

  const answersPayload = {}
  for (const [qId, ans] of Object.entries(state.answers)) {
    const q = state.questions.core_questions.find(q => q.id === qId)
    if (q) answersPayload[qId] = q.options[ans.optionIdx].scores
  }

  try {
    const res = await fetch('/api/reveal', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        animal_id: state.totem.id,
        name: state.name,
        answers: answersPayload,
      }),
    })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json()
    state.resultUUID = data.uuid
    state.imageURL = data.image_url || null
  } catch (e) {
    console.error('reveal error:', e)
  }
  transition('complete')
}

async function loadSharedResult(uuid) {
  try {
    const res = await fetch(`/api/result/${uuid}`)
    if (!res.ok) { transition('intro'); return }
    const data = await res.json()
    state.totem  = state.animals.animals.find(a => a.id === data.animal_id)
    state.shadow = state.animals.animals.find(a => a.id === state.totem?.shadow_animal) || null
    state.name      = data.name
    state.imageURL  = data.image_url || null
    state.resultUUID = data.uuid
    transition('complete')
  } catch {
    transition('intro')
  }
}

// ── Render ────────────────────────────────────────────────────────────────────

function render() {
  const app = document.getElementById('app')
  const map = {
    loading:     renderLoading,
    intro:       renderIntro,
    quiz:        renderQuiz,
    tiebreaker:  renderTiebreaker,
    reveal:      renderReveal,
    name:        renderName,
    generating:  renderGenerating,
    complete:    renderComplete,
  }
  app.innerHTML = (map[state.phase] || renderLoading)()
  bindEvents()
}

// ── Screens ───────────────────────────────────────────────────────────────────

function renderLoading() {
  return `<div class="flex items-center justify-center min-h-screen">
    <span class="font-ritual text-amber-400 text-2xl animate-pulse">…</span>
  </div>`
}

function renderIntro() {
  return `<div class="flex flex-col items-center justify-center min-h-screen px-6 text-center fade-in">
    <div class="text-7xl mb-8">🌙</div>
    <h1 class="font-ritual text-5xl md:text-6xl text-amber-400 mb-4 tracking-widest">TOTEM</h1>
    <p class="text-stone-400 text-lg max-w-sm mb-12 leading-relaxed">
      Découvrez l'animal guide qui habite en vous — votre force, votre ombre, votre chemin.
    </p>
    <button data-a="start"
      class="px-10 py-4 bg-amber-500 hover:bg-amber-400 text-stone-950 font-semibold tracking-widest uppercase text-sm transition-colors">
      Commencer
    </button>
  </div>`
}

function renderQuiz() {
  const qs = state.questions.core_questions
  const q  = qs[state.currentIdx]
  const pct = ((state.currentIdx + 1) / qs.length * 100).toFixed(0)
  const groupLabels = {
    element: 'Élément', introvert_extrovert: 'Énergie sociale',
    intuition_sensing: 'Perception', thinking_feeling: 'Décision',
    structure_freedom: 'Organisation',
  }
  return `<div class="flex flex-col min-h-screen px-6 py-12 max-w-2xl mx-auto fade-in">
    <div class="mb-8">
      <div class="flex justify-between text-stone-500 text-xs mb-2">
        <span>${state.currentIdx + 1} / ${qs.length}</span>
        <span>${groupLabels[q.group] || q.group}</span>
      </div>
      <div class="h-px bg-stone-800">
        <div class="h-px bg-amber-500 transition-all duration-500" style="width:${pct}%"></div>
      </div>
    </div>
    <div class="flex-1 flex flex-col justify-center">
      <p class="text-2xl md:text-3xl text-stone-100 leading-relaxed mb-10">${q.text}</p>
      <div class="space-y-3">
        ${q.options.map((opt, i) => `
          <button data-a="answer" data-idx="${i}"
            class="w-full text-left px-6 py-4 bg-stone-900 hover:bg-stone-800 border border-stone-800 hover:border-amber-600 transition-all text-stone-300 hover:text-stone-100">
            <span class="text-amber-600 mr-3">${opt.id.toUpperCase()}.</span>${escapeHtml(opt.text)}
          </button>`).join('')}
      </div>
    </div>
  </div>`
}

function renderTiebreaker() {
  const q = state.tiebreakerQ
  return `<div class="flex flex-col min-h-screen px-6 py-12 max-w-2xl mx-auto fade-in">
    <div class="mb-8">
      <p class="text-stone-500 text-xs tracking-widest uppercase">Question de discernement</p>
      <div class="h-px bg-amber-800 mt-2"></div>
    </div>
    <div class="flex-1 flex flex-col justify-center">
      <p class="text-2xl md:text-3xl text-stone-100 leading-relaxed mb-10">${q.text}</p>
      <div class="space-y-3">
        ${q.options.map((opt, i) => `
          <button data-a="tiebreaker" data-idx="${i}"
            class="w-full text-left px-6 py-4 bg-stone-900 hover:bg-stone-800 border border-stone-800 hover:border-amber-600 transition-all text-stone-300 hover:text-stone-100">
            ${escapeHtml(opt.text)}
          </button>`).join('')}
      </div>
    </div>
  </div>`
}

function renderReveal() {
  const a = state.totem
  return `<div class="max-w-2xl mx-auto px-6 py-16 fade-in">
    <div class="text-center mb-16">
      <p class="text-stone-500 text-xs tracking-widest uppercase mb-6">Votre animal totem</p>
      <div class="text-8xl mb-6">${a.emoji}</div>
      <h2 class="font-ritual text-4xl md:text-5xl text-amber-400 mb-3">${a.name}</h2>
      <p class="text-stone-400 italic">${a.archetype_jung}</p>
    </div>
    ${renderProfile(a, state.shadow)}
    <div class="text-center space-y-3 mt-12">
      <button data-a="go-name"
        class="w-full sm:w-auto px-10 py-4 bg-amber-500 hover:bg-amber-400 text-stone-950 font-semibold tracking-widest uppercase text-sm transition-colors">
        Créer mon image personnalisée
      </button>
      <p class="text-stone-600 text-xs">
        ou <button data-a="skip-image" class="underline hover:text-stone-400 transition-colors">continuer sans image</button>
      </p>
    </div>
  </div>`
}

// Shared profile block used by both renderReveal and renderComplete.
function renderProfile(a, sh) {
  return `
    <div class="border-l-2 border-amber-600 pl-6 mb-12">
      <p class="text-stone-300 text-lg leading-relaxed">${a.medicine}</p>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-12">
      <div class="bg-stone-900 p-6">
        <h3 class="text-amber-400 text-xs tracking-widest uppercase mb-4">Forces</h3>
        <ul class="space-y-2">${a.traits.strengths.map(s =>
          `<li class="text-stone-300 text-sm">— ${escapeHtml(s)}</li>`).join('')}</ul>
      </div>
      <div class="bg-stone-900 p-6">
        <h3 class="text-stone-500 text-xs tracking-widest uppercase mb-4">Ombres</h3>
        <ul class="space-y-2">${a.traits.shadows.map(s =>
          `<li class="text-stone-400 text-sm">— ${escapeHtml(s)}</li>`).join('')}</ul>
      </div>
    </div>

    <div class="text-center mb-12">
      <p class="text-stone-500 text-xs tracking-widest uppercase mb-3">Thème de vie</p>
      <p class="text-stone-200 text-lg italic">"${escapeHtml(a.traits.life_theme)}"</p>
    </div>

    ${sh ? `<div class="border border-stone-800 p-6 mb-6">
      <p class="text-stone-500 text-xs tracking-widest uppercase mb-3">Animal ombre</p>
      <div class="flex items-center gap-4">
        <span class="text-4xl">${sh.emoji}</span>
        <div>
          <p class="text-stone-200 font-medium">${sh.name}</p>
          <p class="text-stone-500 text-sm italic">${sh.archetype_jung}</p>
        </div>
      </div>
    </div>` : ''}

    <div class="border border-stone-800 mb-6">
      <button data-a="toggle-lover"
        class="w-full flex justify-between items-center p-6 hover:bg-stone-900 transition-colors">
        <span class="text-stone-400 text-xs tracking-widest uppercase">Profil amoureux</span>
        <span id="lover-icon" class="text-amber-600 text-xs">+ voir</span>
      </button>
      <div id="lover-panel" class="hidden px-6 pb-6 space-y-5">
        <p class="text-stone-400 text-sm italic border-b border-stone-800 pb-4">${escapeHtml(a.lover_profile.style)}</p>
        ${loverRow('Forces', a.lover_profile.strengths)}
        ${loverRow('Attention', a.lover_profile.watch_out)}
        ${loverRow('Au lit', a.lover_profile.in_bed)}
        ${loverRow('Rédhibitoire', a.lover_profile.dealbreaker)}
      </div>
    </div>`
}

function loverRow(label, text) {
  return `<div>
    <p class="text-stone-500 text-xs tracking-wider uppercase mb-1">${label}</p>
    <p class="text-stone-300 text-sm leading-relaxed">${escapeHtml(text)}</p>
  </div>`
}

function renderName() {
  return `<div class="flex flex-col items-center justify-center min-h-screen px-6 text-center fade-in">
    <div class="text-5xl mb-8">${state.totem.emoji}</div>
    <h2 class="font-ritual text-3xl text-amber-400 mb-4">Personnalisez votre révélation</h2>
    <p class="text-stone-400 max-w-sm mb-10 leading-relaxed">
      Votre prénom sera tissé dans l'image de votre totem.
    </p>
    <div class="w-full max-w-sm space-y-3">
      <input id="name-input" type="text" placeholder="Votre prénom" autocomplete="given-name"
        class="w-full bg-stone-900 border border-stone-700 focus:border-amber-500 text-stone-100 px-6 py-4 text-center text-lg outline-none placeholder-stone-600 transition-colors" />
      <button data-a="submit-name"
        class="w-full px-8 py-4 bg-amber-500 hover:bg-amber-400 text-stone-950 font-semibold tracking-widest uppercase text-sm transition-colors">
        Générer mon image
      </button>
      <button data-a="skip-name"
        class="w-full py-3 text-stone-500 hover:text-stone-300 text-sm transition-colors">
        Passer cette étape
      </button>
    </div>
  </div>`
}

function renderGenerating() {
  return `<div class="flex flex-col items-center justify-center min-h-screen px-6 text-center fade-in">
    <div class="text-6xl mb-8 animate-pulse">${state.totem.emoji}</div>
    <p class="font-ritual text-2xl text-amber-400 mb-4">L'image se tisse…</p>
    <p class="text-stone-500 text-sm">Cela prend quelques instants.</p>
  </div>`
}

function renderComplete() {
  const a = state.totem
  const shareURL = state.resultUUID
    ? `${window.location.origin}/result/${state.resultUUID}`
    : null

  const nameHeader = state.name
    ? `<p class="text-stone-500 text-xs tracking-widest uppercase mb-6">Révélation de ${escapeHtml(state.name)}</p>`
    : ''

  return `<div class="max-w-2xl mx-auto px-6 py-16 fade-in">
    <div class="text-center mb-12">
      ${nameHeader}
      <div class="text-8xl mb-6">${a.emoji}</div>
      <h2 class="font-ritual text-4xl md:text-5xl text-amber-400 mb-3">${a.name}</h2>
      <p class="text-stone-400 italic">${a.archetype_jung}</p>
    </div>

    ${state.imageURL
      ? `<img src="${escapeHtml(state.imageURL)}" alt="Votre totem ${escapeHtml(a.name)}"
           class="w-full mb-12" />`
      : ''
    }

    ${renderProfile(a, state.shadow)}

    ${shareURL ? `<div class="bg-stone-900 border border-stone-800 p-5 mb-8">
      <p class="text-stone-500 text-xs tracking-widest uppercase mb-2">Lien à partager</p>
      <p class="text-stone-300 text-sm break-all font-mono mb-3">${shareURL}</p>
      <button data-a="copy" data-url="${shareURL}"
        class="px-5 py-2 border border-stone-700 hover:border-amber-600 text-stone-400 hover:text-amber-400 text-xs tracking-widest uppercase transition-colors">
        Copier le lien
      </button>
    </div>` : ''}

    <div class="text-center">
      <button data-a="restart"
        class="text-stone-600 hover:text-stone-400 text-sm underline transition-colors">
        Découvrir mon propre totem
      </button>
    </div>
  </div>`
}

// ── Event binding ─────────────────────────────────────────────────────────────

function bindEvents() {
  document.querySelectorAll('[data-a]').forEach(el => {
    el.addEventListener('click', onAction)
  })

  // Enter key on name input
  const input = document.getElementById('name-input')
  if (input) {
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter') handleSubmitName()
    })
  }
}

function onAction(e) {
  const a = e.currentTarget.dataset.a
  switch (a) {
    case 'start':
      state.currentIdx = 0
      state.answers    = {}
      transition('quiz')
      break
    case 'answer':
      answerCore(
        state.questions.core_questions[state.currentIdx].id,
        parseInt(e.currentTarget.dataset.idx)
      )
      break
    case 'tiebreaker':
      answerTiebreaker(parseInt(e.currentTarget.dataset.idx))
      break
    case 'toggle-lover': {
      const panel = document.getElementById('lover-panel')
      const icon  = document.getElementById('lover-icon')
      panel.classList.toggle('hidden')
      icon.textContent = panel.classList.contains('hidden') ? '+ voir' : '− cacher'
      break
    }
    case 'go-name':
      transition('name')
      break
    case 'skip-image':
      transition('complete')
      break
    case 'submit-name':
      handleSubmitName()
      break
    case 'skip-name':
      transition('complete')
      break
    case 'copy': {
      const url = e.currentTarget.dataset.url
      navigator.clipboard.writeText(url).then(() => {
        e.currentTarget.textContent = 'Copié !'
        setTimeout(() => { e.currentTarget.textContent = 'Copier le lien' }, 2000)
      })
      break
    }
    case 'restart':
      Object.assign(state, {
        phase: 'intro', currentIdx: 0, answers: {},
        tiebreakerQ: null, totem: null, shadow: null,
        name: '', resultUUID: null, imageURL: null,
      })
      render()
      break
  }
}

function handleSubmitName() {
  const input = document.getElementById('name-input')
  const name  = input?.value?.trim() || ''
  if (name) submitName(name)
}

// ── Utilities ─────────────────────────────────────────────────────────────────

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

// ── Boot ──────────────────────────────────────────────────────────────────────

async function init() {
  try {
    const [qRes, aRes] = await Promise.all([
      fetch('/data/questions.json'),
      fetch('/data/animals.json'),
    ])
    state.questions = await qRes.json()
    state.animals   = await aRes.json()
  } catch (e) {
    console.error('Failed to load data:', e)
    document.getElementById('app').innerHTML =
      `<div class="flex items-center justify-center min-h-screen text-red-400">
         Erreur de chargement. Rechargez la page.
       </div>`
    return
  }

  // Check if we're on a shared result route
  const uuidMatch = window.location.pathname.match(/^\/result\/([0-9a-f]+)$/)
  if (uuidMatch) {
    await loadSharedResult(uuidMatch[1])
  } else {
    transition('intro')
  }
}

init()
