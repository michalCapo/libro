package components

import (
	"slices"
	"testing"
)

func TestMergeEnvironmentOverridesInheritedValues(t *testing.T) {
	got := mergeEnvironment([]string{"PATH=/bin", "OPENROUTER_API_KEY=old", "KEEP=yes"}, []string{"OPENROUTER_API_KEY=new", "MODEL=test", "MODEL=final"})
	want := []string{"PATH=/bin", "KEEP=yes", "OPENROUTER_API_KEY=new", "MODEL=final"}
	if !slices.Equal(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}
