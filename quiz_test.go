package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// ── Shared types and helpers ──────────────────────────────────────────────────

type quizProfile struct {
	IO float64 `json:"introvert_extrovert"`
	IS float64 `json:"intuition_sensing"`
	TF float64 `json:"thinking_feeling"`
	SF float64 `json:"structure_freedom"`
}

func (p quizProfile) dim(name string) float64 {
	switch name {
	case "introvert_extrovert":
		return p.IO
	case "intuition_sensing":
		return p.IS
	case "thinking_feeling":
		return p.TF
	case "structure_freedom":
		return p.SF
	}
	return 0.5
}

type quizAnimal struct {
	ID      string      `json:"id"`
	Element string      `json:"element"`
	Profile quizProfile `json:"personality_profile"`
}

type quizRanked struct {
	id   string
	dist float64
}

// Must mirror ELEM_VEC in frontend/js/app.js.
var quizElemVecs = map[string][4]float64{
	"terre":     {1, 0, 0, 0},
	"air":       {0, 1, 0, 0},
	"feu":       {0, 0, 1, 0},
	"eau":       {0, 0, 0, 1},
	"terre/feu": {0.5, 0, 0.5, 0},
	"eau/terre": {0.5, 0, 0, 0.5},
	"air/terre": {0.5, 0.5, 0, 0},
}

var quizDefaultElem = [4]float64{0.25, 0.25, 0.25, 0.25}

func quizElemVec(element string) [4]float64 {
	if v, ok := quizElemVecs[element]; ok {
		return v
	}
	return quizDefaultElem
}

func quizSq(x float64) float64 { return x * x }

func quizElemDist(a, b [4]float64) float64 {
	sum := 0.0
	for i := range a {
		sum += quizSq(a[i] - b[i])
	}
	return sum // squared distance, no sqrt needed for comparisons
}

func quizRankAnimals(animals []quizAnimal, uElem [4]float64, up quizProfile, w float64) []quizRanked {
	results := make([]quizRanked, len(animals))
	for i, a := range animals {
		aElem := quizElemVec(a.Element)
		elemSq := quizElemDist(uElem, aElem)
		persSq := quizSq(up.IO-a.Profile.IO) +
			quizSq(up.IS-a.Profile.IS) +
			quizSq(up.TF-a.Profile.TF) +
			quizSq(up.SF-a.Profile.SF)
		results[i] = quizRanked{a.ID, math.Sqrt(w*w*elemSq + persSq)}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].dist < results[j].dist
	})
	return results
}

func quizFailReport(t *testing.T, targetID string, results []quizRanked) {
	t.Helper()
	var b strings.Builder
	for i, r := range results {
		marker := "   "
		if r.id == targetID {
			marker = " → "
		}
		fmt.Fprintf(&b, "%srank %2d: %-12s  dist=%.4f\n", marker, i+1, r.id, r.dist)
	}
	t.Error(b.String())
}

func quizParseAnimals(t *testing.T) []quizAnimal {
	t.Helper()
	var db struct {
		Animals []quizAnimal `json:"animals"`
	}
	if err := json.Unmarshal(animalsJSON, &db); err != nil {
		t.Fatalf("parse animals.json: %v", err)
	}
	return db.Animals
}

// ── Test 1: ideal synthetic scores ───────────────────────────────────────────

// TestAllAnimalsAccessible verifies that every animal is the closest match when
// the user's scores exactly equal that animal's profile (distance = 0).
// A failure means the animal's 5D position is dominated — it can never win.
func TestAllAnimalsAccessible(t *testing.T) {
	animals := quizParseAnimals(t)

	var meta struct {
		Meta struct {
			ElementWeight float64 `json:"element_weight"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(questionsJSON, &meta); err != nil {
		t.Fatalf("parse questions.json: %v", err)
	}
	w := meta.Meta.ElementWeight

	for _, target := range animals {
		t.Run(target.ID, func(t *testing.T) {
			results := quizRankAnimals(animals, quizElemVec(target.Element), target.Profile, w)
			if results[0].id != target.ID {
				t.Errorf("%s is not rank 1 with its own ideal scores:", target.ID)
				quizFailReport(t, target.ID, results)
			}
		})
	}
}

// ── Test 2: reachable via real quiz answer options ────────────────────────────

// TestAllAnimalsReachableViaQuiz verifies that every animal can be reached
// through the actual discrete answer options in questions.json.
// For each animal it greedily picks the option per question that best matches
// that animal's profile, then checks the animal wins (or reaches a tiebreaker).
// A failure means no real answer combination can select that animal.
func TestAllAnimalsReachableViaQuiz(t *testing.T) {
	animals := quizParseAnimals(t)

	var questionsDB struct {
		Meta struct {
			ElementWeight       float64 `json:"element_weight"`
			TiebreakerThreshold float64 `json:"tiebreaker_threshold"`
		} `json:"meta"`
		CoreQuestions []struct {
			Group   string `json:"group"`
			Options []struct {
				Scores map[string]json.RawMessage `json:"scores"`
			} `json:"options"`
		} `json:"core_questions"`
		TiebreakerPairs []struct {
			Animals [2]string `json:"animals"`
		} `json:"tiebreaker_pairs"`
	}
	if err := json.Unmarshal(questionsJSON, &questionsDB); err != nil {
		t.Fatalf("parse questions.json: %v", err)
	}
	w := questionsDB.Meta.ElementWeight
	threshold := questionsDB.Meta.TiebreakerThreshold

	hasTiebreaker := func(id1, id2 string) bool {
		for _, tb := range questionsDB.TiebreakerPairs {
			if (tb.Animals[0] == id1 && tb.Animals[1] == id2) ||
				(tb.Animals[0] == id2 && tb.Animals[1] == id1) {
				return true
			}
		}
		return false
	}

	for _, target := range animals {
		t.Run(target.ID, func(t *testing.T) {
			tElem := quizElemVec(target.Element)

			// Collect element option vectors per question (for enumeration below).
			// For float dimensions, greedy per-question is optimal since each
			// question contributes independently to its dimension's average.
			var elemOptSets [][]([4]float64)
			dimSums := map[string]float64{}
			dimCounts := map[string]int{}

			for _, q := range questionsDB.CoreQuestions {
				if q.Group == "element" {
					var vecs [][4]float64
					for _, opt := range q.Options {
						raw, ok := opt.Scores["element"]
						if !ok {
							continue
						}
						var elemStr string
						if err := json.Unmarshal(raw, &elemStr); err != nil {
							continue
						}
						vecs = append(vecs, quizElemVec(elemStr))
					}
					elemOptSets = append(elemOptSets, vecs)
				} else {
					dim := q.Group
					want := target.Profile.dim(dim)
					bestDist := math.MaxFloat64
					bestVal := 0.5
					for _, opt := range q.Options {
						raw, ok := opt.Scores[dim]
						if !ok {
							continue
						}
						var v float64
						if err := json.Unmarshal(raw, &v); err != nil {
							continue
						}
						if d := math.Abs(v - want); d < bestDist {
							bestDist = d
							bestVal = v
						}
					}
					dimSums[dim] += bestVal
					dimCounts[dim]++
				}
			}

			// Enumerate all element option combinations to find the average
			// element vector closest to the target. Greedy per-question fails
			// for dual-element animals (e.g. terre/feu needs one "terre" answer
			// and one "feu" answer, not the same option twice).
			uElem := quizDefaultElem
			if len(elemOptSets) > 0 {
				bestElemDist := math.MaxFloat64
				var tryElem func(qi int, sum [4]float64)
				tryElem = func(qi int, sum [4]float64) {
					if qi == len(elemOptSets) {
						avg := sum
						for j := range avg {
							avg[j] /= float64(len(elemOptSets))
						}
						if d := quizElemDist(avg, tElem); d < bestElemDist {
							bestElemDist = d
							uElem = avg
						}
						return
					}
					for _, v := range elemOptSets[qi] {
						var next [4]float64
						for j := range next {
							next[j] = sum[j] + v[j]
						}
						tryElem(qi+1, next)
					}
				}
				tryElem(0, [4]float64{})
			}

			// Compute user float scores (average per dimension).
			avg := func(dim string) float64 {
				if dimCounts[dim] == 0 {
					return 0.5
				}
				return dimSums[dim] / float64(dimCounts[dim])
			}
			up := quizProfile{
				IO: avg("introvert_extrovert"),
				IS: avg("intuition_sensing"),
				TF: avg("thinking_feeling"),
				SF: avg("structure_freedom"),
			}

			results := quizRankAnimals(animals, uElem, up, w)

			rank1 := results[0].id == target.ID
			rank2WithTB := len(results) > 1 &&
				results[1].id == target.ID &&
				(results[1].dist-results[0].dist) < threshold &&
				hasTiebreaker(results[0].id, target.ID)

			if !rank1 && !rank2WithTB {
				t.Errorf("%s not reachable via quiz (greedy best options per question):", target.ID)
				quizFailReport(t, target.ID, results)
			}
		})
	}
}
