package components

import (
	"encoding/json"
	"fmt"
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

// HideJS returns the JS snippet that hides an element by ID via the "hidden"
// class. Used as the click handler for dialog backdrops and close buttons.
func HideJS(id string) string {
	return fmt.Sprintf("document.getElementById('%s').classList.add('hidden');", id)
}
