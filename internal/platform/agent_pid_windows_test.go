//go:build windows

package platform

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestAgentPIDRecordWillNotTerminateCurrentProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.pid")
	if err := WriteAgentPID(path); err != nil {
		t.Fatal(err)
	}
	defer ClearAgentPID(path, os.Getpid())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := RecoverStaleAgent(Paths{AgentPID: path}, exe)
	if err != nil {
		t.Fatal(err)
	}
	if recovered {
		t.Fatal("refused to recover the current process")
	}
	info, err := readAgentPID(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.PID != os.Getpid() || info.Started == 0 || info.Executable != exe {
		t.Fatalf("unexpected PID record: %+v", info)
	}
}

func TestRecoverStaleAgentTerminatesOnlyRecordedProcess(t *testing.T) {
	exe, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Skip("cmd.exe is unavailable")
	}
	cmd := exec.Command(exe, "/c", "timeout /t 120 >nul")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	var created, exited, kernel, user windows.Filetime
	err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
	_ = windows.CloseHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	started := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	path := filepath.Join(t.TempDir(), "agent.pid")
	data, err := json.Marshal(agentProcess{PID: cmd.Process.Pid, Started: started, Executable: exe})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := RecoverStaleAgent(Paths{AgentPID: path}, exe)
	if err != nil || !recovered {
		t.Fatalf("recovered=%v err=%v", recovered, err)
	}
	err = cmd.Wait()
	waited = true
	if err == nil {
		t.Fatal("terminated child unexpectedly exited cleanly")
	}
}
