package tunnel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// spawnerEnv makes TestSpawner start a sleeper and write its pid to the file
// that the variable names, as ssh starts a ProxyCommand.
const spawnerEnv = "LAZYFWD_TEST_SPAWNER"

func TestSpawner(t *testing.T) {
	pidFile := os.Getenv(spawnerEnv)
	if pidFile == "" {
		t.Skip("helper process for other tests")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestSleeper$")
	child.Env = append(os.Environ(), sleeperEnv+"=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Minute)
}

// Stopping a forward must end what its ssh started too, such as the ssh of
// a ProxyJump.
func TestKillEndsWhatTheProcessStarted(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSpawner$")
	cmd.Env = append(os.Environ(), spawnerEnv+"="+pidFile)
	process, err := startProcess(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var childPID int
	deadline := time.Now().Add(10 * time.Second)
	for childPID == 0 && time.Now().Before(deadline) {
		content, _ := os.ReadFile(pidFile)
		childPID, _ = strconv.Atoi(string(content))
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 || !isProcessAlive(childPID) {
		process.kill()
		t.Fatalf("the spawner's child did not start (pid %d)", childPID)
	}

	process.kill()

	deadline = time.Now().Add(5 * time.Second)
	for isProcessAlive(childPID) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d still runs after its job was closed", childPID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
