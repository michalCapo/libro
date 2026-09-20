package libro

import (
	"strings"
	"testing"
)

func TestProjectDialogFilterSurvivesDialogReplacement(t *testing.T) {
	script := projectDialogJS("test-session")

	if !strings.Contains(script, "document.addEventListener('input'") {
		t.Fatal("project filtering must use a document listener so replacing the dialog does not detach it")
	}
	if strings.Contains(script, "inp.addEventListener('input',filter)") {
		t.Fatal("project filtering is still bound to the replaceable input element")
	}
}
