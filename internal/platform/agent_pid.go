package platform

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type agentProcess struct {
	PID        int    `json:"pid"`
	Started    uint64 `json:"started"`
	Executable string `json:"executable"`
}

func WriteAgentPID(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	started, err := processStartTime()
	if err != nil {
		return err
	}
	data, err := json.Marshal(agentProcess{PID: os.Getpid(), Started: started, Executable: exe})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func ClearAgentPID(path string, pid int) {
	info, err := readAgentPID(path)
	if err == nil && info.PID == pid {
		_ = os.Remove(path)
	}
}

func readAgentPID(path string) (agentProcess, error) {
	var info agentProcess
	data, err := os.ReadFile(path)
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(data, &info)
	if err == nil && (info.PID < 1 || info.Executable == "") {
		err = errors.New("invalid agent PID record")
	}
	return info, err
}

func agentPIDPath(p Paths) string {
	if p.AgentPID != "" {
		return p.AgentPID
	}
	return filepath.Join(p.DataDir, "agent.pid")
}
