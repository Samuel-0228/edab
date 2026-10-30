package version

import (
	"strings"
	"testing"
)

func TestVersionInfo(t *testing.T) {
	info := Info()
	if !strings.Contains(info, "Kerob Automation Engine") {
		t.Errorf("expected Info() to contain 'Kerob Automation Engine', got %s", info)
	}
	if !strings.Contains(info, Version) {
		t.Errorf("expected Info() to contain version %s, got %s", Version, info)
	}
}

func TestVersionShort(t *testing.T) {
	short := Short()
	if short != Version {
		t.Errorf("expected Short() == %s, got %s", Version, short)
	}
}
