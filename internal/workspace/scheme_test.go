package workspace

import "testing"

func TestLocalURLFollowsTargetPort(t *testing.T) {
	for _, test := range []struct {
		forward Forward
		url     string
		isWeb   bool
	}{
		{Forward{Direction: Local, BindPort: 9000, Target: Addr{"localhost", 8080}}, "http://localhost:9000", true},
		{Forward{Direction: Local, BindPort: 8443, Target: Addr{"localhost", 443}}, "https://localhost:8443", true},
		{Forward{Direction: Local, BindPort: 5433, Target: Addr{"db", 5432}}, "", false},
		{Forward{Direction: Remote, BindPort: 9000, Target: Addr{"localhost", 8443}}, "https://localhost:8443", true},
	} {
		if url, isWeb := test.forward.LocalURL(); url != test.url || isWeb != test.isWeb {
			t.Errorf("%v: got %q, %v, want %q, %v", test.forward, url, isWeb, test.url, test.isWeb)
		}
	}
}
