package main

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// loadDotEnv reads a .env file and sets any variable that is not already
// present in the environment.  Missing file is silently ignored.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file is fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Strip optional surrounding quotes (" or ')
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Existing env vars take priority over the file
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				log.Printf(".env: could not set %s: %v", key, err)
			}
		}
	}
}
