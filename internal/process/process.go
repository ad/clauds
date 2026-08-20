package process

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Manager interface {
	Launch(appBundle, dataDir string) error
	IsRunning(dataDir string) (bool, error)
	Activate(appBundle string) error
	Quit(dataDir string, timeout time.Duration) error
}

type ExecManager struct{}

func (e *ExecManager) Launch(appBundle, dataDir string) error {
	cmd := exec.Command("open", "-n", "-a", appBundle, "--args", fmt.Sprintf("--user-data-dir=%s", dataDir))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launching Claude instance via `open`: %w", err)
	}
	return nil
}

func (e *ExecManager) IsRunning(dataDir string) (bool, error) {
	pids, err := e.pidsForDataDir(dataDir)
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

func (e *ExecManager) Activate(appBundle string) error {
	cmd := exec.Command("open", "-a", appBundle)
	_ = cmd.Run()
	return nil
}

func (e *ExecManager) Quit(dataDir string, timeout time.Duration) error {
	pids, err := e.pidsForDataDir(dataDir)
	if err != nil {
		return err
	}

	if len(pids) == 0 {
		return nil
	}

	for _, pid := range pids {
		cmd := exec.Command("kill", "-TERM", fmt.Sprintf("%d", pid))
		_ = cmd.Run()
	}

	deadline := time.Now().Add(timeout)
	for {
		running, _ := e.pidsForDataDir(dataDir)
		if len(running) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			for _, pid := range running {
				cmd := exec.Command("kill", "-KILL", fmt.Sprintf("%d", pid))
				_ = cmd.Run()
			}
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (e *ExecManager) pidsForDataDir(dataDir string) ([]int, error) {
	needle := fmt.Sprintf("--user-data-dir=%s", dataDir)
	out, err := exec.Command("pgrep", "-f", "--", needle).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("running pgrep: %w", err)
	}

	me := os.Getpid()
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(line, "%d", &pid); err == nil && pid != me {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
