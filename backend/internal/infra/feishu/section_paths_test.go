package feishu

import (
	"reflect"
	"testing"
)

func TestCitationSectionPathsDisambiguateDuplicateAndCollidingLabels(t *testing.T) {
	got := citationSectionPaths(
		[]string{"Status", "Status", "Status [sh1]"},
		[]string{"sh1", "sh2", "sh3"},
	)
	want := []string{"Status [sh1]", "Status [sh2]", "Status [sh1] [sh3]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("citationSectionPaths() = %#v, want %#v", got, want)
	}
}
