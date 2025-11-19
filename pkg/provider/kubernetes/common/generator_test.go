package main

import (
	"reflect"
	"testing"
)

func TestGenerateOutput(t *testing.T) {
	groupToKinds := map[string][]string{
		"group1": {"kind1", "kind2"},
		"group2": {"kind3", "kind4"},
	}
	expected := map[string]string{
		"kind1": "group1",
		"kind2": "group1",
		"kind3": "group2",
		"kind4": "group2",
	}
	output := generateOutput(groupToKinds)
	if !reflect.DeepEqual(output, expected) {
		t.Errorf("expected %v, got %v", expected, output)
	}
}
