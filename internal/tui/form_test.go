package tui

import (
	"testing"

	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// Changing only a port must be one word to replace: alt+backspace deletes
// back to whitespace, so "localhost:1234" would go whole.
func TestEditFormWritesEachEndAsShortAsItParsesBack(t *testing.T) {
	cases := []struct {
		forward    workspace.Forward
		wantTarget string
		wantBind   string
	}{
		{workspace.Forward{Direction: workspace.Local, Bind: workspace.End{Host: workspace.BindHost, Port: 9000}, Target: workspace.End{Host: "localhost", Port: 1234}}, "1234", "9000"},
		{workspace.Forward{Direction: workspace.Local, Bind: workspace.End{Host: workspace.BindHost, Port: 5433}, Target: workspace.End{Host: "db", Port: 5432}}, "db:5432", "5433"},
		{workspace.Forward{Direction: workspace.Remote, Bind: workspace.End{Path: "/tmp/app.sock"}, Target: workspace.End{Path: "/run/app.sock"}}, "/run/app.sock", "/tmp/app.sock"},
	}
	for _, c := range cases {
		f := newEditForm(c.forward)
		if f.value(0) != c.wantTarget || f.value(1) != c.wantBind {
			t.Errorf("got %q, %q; want %q, %q", f.value(0), f.value(1), c.wantTarget, c.wantBind)
		}
		targets, err := workspace.ParseTargets(f.value(0))
		bind, bindErr := workspace.ParseBind(f.value(1))
		if err != nil || bindErr != nil || targets[0] != c.forward.Target || bind != c.forward.Bind {
			t.Errorf("%q, %q parse back to %v, %v (%v, %v)", f.value(0), f.value(1), targets, bind, err, bindErr)
		}
	}
}
