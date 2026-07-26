package checks

import (
	"testing"

	"github.com/francomano/proxydoctor/core/engine"
)

func TestRegisterDefaultsReturnsDuplicateError(t *testing.T) {
	registry := engine.NewCheckRegistry()
	if err := RegisterDefaults(registry); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := RegisterDefaults(registry); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}
