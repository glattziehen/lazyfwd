package tunnel

import (
	"strings"
	"testing"
)

// The reason a forward failed must be ssh's error, not -v's debugging or the
// transfer statistics that ssh prints as it exits.
func TestLastSignificantLineSkipsVerboseNoise(t *testing.T) {
	log := strings.Join([]string{
		"debug1: Authenticating to 127.0.0.1:2223 as 'root'",
		"Authenticated to 127.0.0.1 ([127.0.0.1]:2223) using \"publickey\".",
		"Connection to 127.0.0.1 closed by remote host.",
		"Transferred: sent 3240, received 2380 bytes, in 14.1 seconds",
		"Bytes per second: sent 229.9, received 168.9",
		"debug1: Exit status -1",
	}, "\r\n")
	if got := lastSignificantLine(log); got != "Connection to 127.0.0.1 closed by remote host." {
		t.Errorf("got %q", got)
	}
}

// On Windows a login cannot help a host that wants a password, but it can
// accept an unknown host key.
func TestExplainFailureOffersALoginOnlyForAHostKey(t *testing.T) {
	reason, needsLogin := explainFailure("nobody@127.0.0.1: Permission denied (publickey,password).", nil)
	if needsLogin || !strings.HasPrefix(reason, "nobody@127.0.0.1: Permission denied (publickey,password); on Windows lazyfwd needs a key") {
		t.Errorf("password host: %q, %v", reason, needsLogin)
	}
	reason, needsLogin = explainFailure("No ED25519 host key is known for [127.0.0.1]:2223 and you have requested strict checking.\nHost key verification failed.", nil)
	if !needsLogin || reason != "Host key verification failed." {
		t.Errorf("unknown host key: %q, %v", reason, needsLogin)
	}
}

// ssh -v logs a line per connection through a forward for as long as the
// forward runs; the log keeps a bounded tail and still sees its marker.
func TestSSHLogMarksAndStaysBounded(t *testing.T) {
	log := newSSHLog(remoteForwardReady)
	_, _ = log.Write([]byte("debug1: remote forward success for: listen 18085, connect 127.0.0.1:18081\r\ndebug1: forwarding_succ"))
	_, _ = log.Write([]byte("ess: all expected forwarding replies received\r\n"))
	select {
	case <-log.marked:
	default:
		t.Fatal("the marker split across writes went unseen")
	}
	for range 10 * sshLogLines {
		_, _ = log.Write([]byte("channel 1: open failed: connect failed: Connection refused\n"))
	}
	if lines := strings.Count(log.String(), "\n"); lines > sshLogLines {
		t.Errorf("the log keeps %d lines", lines)
	}
}
