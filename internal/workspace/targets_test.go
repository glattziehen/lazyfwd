package workspace

import (
	"slices"
	"testing"
)

func TestParseTargets(t *testing.T) {
	cases := map[string][]Addr{
		"4966-4968":     {{"localhost", 4966}, {"localhost", 4967}, {"localhost", 4968}},
		"db:5432, 6379": {{"db", 5432}, {"localhost", 6379}},
		"[::1]:80":      {{"::1", 80}},
	}
	for text, want := range cases {
		got, err := ParseTargets(text)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("ParseTargets(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
}

func TestParseTargetsRejectsInvalidInput(t *testing.T) {
	for _, text := range []string{"", "0", "70000", "x", "4968-4966", ":80", "1-1000"} {
		if _, err := ParseTargets(text); err == nil {
			t.Errorf("ParseTargets(%q) succeeded, want error", text)
		}
	}
}
