package workspace

import "testing"

func TestLocalURLFollowsTargetPort(t *testing.T) {
	for _, test := range []struct {
		forward Forward
		url     string
		isWeb   bool
	}{
		{Forward{Direction: Local, Bind: End{Host: BindHost, Port: 9000}, Target: End{Host: "localhost", Port: 8080}}, "http://localhost:9000", true},
		{Forward{Direction: Local, Bind: End{Host: BindHost, Port: 8443}, Target: End{Host: "localhost", Port: 443}}, "https://localhost:8443", true},
		{Forward{Direction: Local, Bind: End{Host: BindHost, Port: 5433}, Target: End{Host: "db", Port: 5432}}, "", false},
		{Forward{Direction: Remote, Bind: End{Host: BindHost, Port: 9000}, Target: End{Host: "localhost", Port: 8443}}, "https://localhost:8443", true},
		// The Docker API speaks HTTP, so a TCP local end to a socket target links.
		{Forward{Direction: Local, Bind: End{Host: BindHost, Port: 2375}, Target: End{Path: "/var/run/docker.sock"}}, "http://localhost:2375", true},
		{Forward{Direction: Local, Bind: End{Path: "/tmp/app.sock"}, Target: End{Host: "localhost", Port: 8080}}, "", false},
		{Forward{Direction: Remote, Bind: End{Host: BindHost, Port: 9000}, Target: End{Path: "/tmp/app.sock"}}, "", false},
	} {
		if url, isWeb := test.forward.LocalURL(); url != test.url || isWeb != test.isWeb {
			t.Errorf("%v: got %q, %v, want %q, %v", test.forward, url, isWeb, test.url, test.isWeb)
		}
	}
}
