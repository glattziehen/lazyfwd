package tunnel

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// job is a Windows job object that holds one ssh process and every process
// it starts, such as a ProxyCommand or ProxyJump ssh. Windows kills them all
// when the job's last handle closes: when lazyfwd calls close, and when
// lazyfwd ends however it ends, a crash included. Only lazyfwd holds the
// handle; Go passes no handles to children unless asked.
type job struct {
	handle    windows.Handle
	closeOnce sync.Once
}

// startInJob starts cmd in a new job. The process starts suspended and
// resumes only once it is in the job, so nothing it starts can escape.
// cmd gets no console window of its own and no ctrl+c from lazyfwd's console.
func startInJob(cmd *exec.Cmd) (*job, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	j := &job{handle: handle}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		j.close()
		return nil, err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW
	if err := cmd.Start(); err != nil {
		j.close()
		return nil, err
	}
	if err := j.adopt(uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		j.close()
		return nil, err
	}
	return j, nil
}

// adopt moves the suspended process pid into the job and resumes it.
func (j *job) adopt(pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	err = windows.AssignProcessToJobObject(j.handle, process)
	_ = windows.CloseHandle(process)
	if err != nil {
		return err
	}
	return resumeThreads(pid)
}

// resumeThreads resumes the threads of pid; a process created suspended has
// one.
func resumeThreads(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := 0
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if err != nil {
			return err
		}
		resumed++
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	if resumed == 0 {
		return errors.New("ssh started without a thread to resume")
	}
	return nil
}

// close kills every process in the job. Closing a closed job is a no-op;
// closing its handle twice could close a handle that reused the number.
func (j *job) close() {
	j.closeOnce.Do(func() { _ = windows.CloseHandle(j.handle) })
}

// jobProcess is a process started in a job of its own.
type jobProcess struct {
	cmd *exec.Cmd
	job *job
	// exited closes once the process has exited and Wait returned err.
	exited chan struct{}
	err    error
}

// startProcess starts cmd in a job of its own and waits for it in the
// background.
func startProcess(cmd *exec.Cmd) (*jobProcess, error) {
	j, err := startInJob(cmd)
	if err != nil {
		return nil, err
	}
	p := &jobProcess{cmd: cmd, job: j, exited: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.exited)
	}()
	return p, nil
}

// kill ends the process and what it started, and waits until it has exited.
func (p *jobProcess) kill() {
	p.job.close()
	<-p.exited
}
