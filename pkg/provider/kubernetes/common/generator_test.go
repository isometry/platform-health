package main

import (
	"reflect"
	"testing"
)

func TestGenerateOutput(t *testing.T) {
	gvToKinds := map[GV][]string{
		{Group: "group1", Version: "v1"}: {"kind1", "kind2"},
		{Group: "group2", Version: "v2"}: {"kind3", "kind4"},
	}
	expected := map[string]GV{
		"kind1": {Group: "group1", Version: "v1"},
		"kind2": {Group: "group1", Version: "v1"},
		"kind3": {Group: "group2", Version: "v2"},
		"kind4": {Group: "group2", Version: "v2"},
	}
	output := generateOutput(gvToKinds)
	if !reflect.DeepEqual(output, expected) {
		t.Errorf("expected %v, got %v", expected, output)
	}
}
