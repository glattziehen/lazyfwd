package workspace

import (
	"slices"
	"testing"
)

func TestSSHArgsCoverEveryEndCombination(t *testing.T) {
	port := End{Host: BindHost, Port: 2375}
	socket := End{Path: "/tmp/app.sock"}
	tcpTarget := End{Host: "localhost", Port: 8080}
	socketTarget := End{Path: "/var/run/docker.sock"}
	cases := []struct {
		direction Direction
		bind      End
		target    End
		want      []string
	}{
		{Local, port, tcpTarget, []string{"-L", "2375:localhost:8080"}},
		{Local, port, socketTarget, []string{"-L", "2375:/var/run/docker.sock"}},
		{Local, socket, tcpTarget, []string{"-L", "/tmp/app.sock:localhost:8080"}},
		{Local, socket, socketTarget, []string{"-L", "/tmp/app.sock:/var/run/docker.sock"}},
		{Remote, port, tcpTarget, []string{"-R", "2375:localhost:8080"}},
		{Remote, port, socketTarget, []string{"-R", "2375:/var/run/docker.sock"}},
		{Remote, socket, tcpTarget, []string{"-R", "/tmp/app.sock:localhost:8080"}},
		{Remote, socket, socketTarget, []string{"-R", "/tmp/app.sock:/var/run/docker.sock"}},
	}
	for _, c := range cases {
		forward := Forward{Direction: c.direction, Bind: c.bind, Target: c.target}
		if got := forward.SSHArgs(); !slices.Equal(got, c.want) {
			t.Errorf("%s %v -> %v: got %v, want %v", c.direction, c.bind, c.target, got, c.want)
		}
	}
}
