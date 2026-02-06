package output

import "testing"

func TestFormatterRegistry(t *testing.T) {
	// JSON, JUnit, and YAML formatters should be registered via init()
	names := FormatNames()
	if len(names) < 3 {
		t.Errorf("expected at least 3 formatters, got %d: %v", len(names), names)
	}

	// Check json formatter exists
	if _, ok := GetFormatter("json"); !ok {
		t.Error("json formatter not registered")
	}

	// Check junit formatter exists
	if _, ok := GetFormatter("junit"); !ok {
		t.Error("junit formatter not registered")
	}

	// Check yaml formatter exists
	if _, ok := GetFormatter("yaml"); !ok {
		t.Error("yaml formatter not registered")
	}

	// Check unknown formatter returns false
	if _, ok := GetFormatter("unknown"); ok {
		t.Error("expected unknown formatter to not exist")
	}
}
