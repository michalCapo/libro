package components

import (
	"encoding/json"
	"log"
)

// JSString returns a JSON-encoded string safe for embedding in JavaScript.
// It includes the surrounding quotes.
func JSString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		log.Printf("components: JSString json.Marshal failed for %q: %v", s, err)
		return `""`
	}
	return string(b)
}
