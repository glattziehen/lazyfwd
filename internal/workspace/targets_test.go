package workspace

import (
	"slices"
	"strings"
	"testing"
)

func TestParseTargets(t *testing.T) {
	cases := map[string][]End{
		"4966-4968":                  {{Host: "localhost", Port: 4966}, {Host: "localhost", Port: 4967}, {Host: "localhost", Port: 4968}},
		"db:5432, 6379":              {{Host: "db", Port: 5432}, {Host: "localhost", Port: 6379}},
		"[::1]:80":                   {{Host: "::1", Port: 80}},
		"/var/run/docker.sock, 8080": {{Path: "/var/run/docker.sock"}, {Host: "localhost", Port: 8080}},
	}
	for text, want := range cases {
		got, err := ParseTargets(text)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("ParseTargets(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
}

func TestParseTargetsRejectsInvalidInput(t *testing.T) {
	for _, text := range []string{
		"", "0", "70000", "x", "4968-4966", ":80", "1-1000",
		"~/app.sock", "run/app.sock", "/tmp/my app.sock", "/tmp/it's.sock", "/" + strings.Repeat("x", 103),
	} {
		if _, err := ParseTargets(text); err == nil {
			t.Errorf("ParseTargets(%q) succeeded, want error", text)
		}
	}
}

func TestParseBind(t *testing.T) {
	cases := map[string]End{
		"":              {},
		"9000":          {Host: BindHost, Port: 9000},
		" /tmp/a.sock ": {Path: "/tmp/a.sock"},
	}
	for text, want := range cases {
		if got, err := ParseBind(text); err != nil || got != want {
			t.Errorf("ParseBind(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
	for _, text := range []string{"~/a.sock", "a.sock/", "0"} {
		if _, err := ParseBind(text); err == nil {
			t.Errorf("ParseBind(%q) succeeded, want error", text)
		}
	}
}
