package main

import (
	"regexp"
	"testing"
)

func TestVersionIsPlainSemver(t *testing.T) {
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version()) {
		t.Fatalf("VERSION must hold MAJOR.MINOR.PATCH without a leading v, got %q", version())
	}
}
