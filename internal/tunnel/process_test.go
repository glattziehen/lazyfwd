package tunnel

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// sleeperEnv makes TestSleeper sleep, so tests can run a process that only
// ends when killed, on every system.
const sleeperEnv = "LAZYFWD_TEST_SLEEPER"

func TestSleeper(t *testing.T) {
	if os.Getenv(sleeperEnv) == "" {
		t.Skip("helper process for other tests")
	}
	time.Sleep(time.Minute)
}

// A master that died must not count as alive, or its forwards would show as
// running.
func TestIsProcessAliveUntilTheProcessExits(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSleeper$")
	cmd.Env = append(os.Environ(), sleeperEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if !isProcessAlive(cmd.Process.Pid) {
		t.Fatal("running process reported dead")
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	if isProcessAlive(cmd.Process.Pid) {
		t.Fatal("killed process reported alive")
	}
}
