package process

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Manager interface {
	Launch(appBundle, dataDir, configDir string) error
	IsRunning(dataDir string) (bool, error)
	Activate(dataDir string) error
	Quit(dataDir string, timeout time.Duration) error
}

type ExecManager struct{}

func (e *ExecManager) Launch(appBundle, dataDir, configDir string) error {
	cmd := exec.Command("open", "-n", "-a", appBundle,
		"--env", "CLAUDE_CONFIG_DIR="+configDir,
		"--args", fmt.Sprintf("--user-data-dir=%s", dataDir))
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

// NSRunningApplication needs no Automation permission, unlike System Events.
const activateJS = `ObjC.import('AppKit');
function run(argv) {
  for (const pid of argv) {
    const app = $.NSRunningApplication.runningApplicationWithProcessIdentifier(Number(pid));
    if (!app.isNil() && Number(app.activationPolicy) === 0) {
      app.unhide;
      app.activateWithOptions(0);
      return;
    }
  }
}`

func (e *ExecManager) Activate(dataDir string) error {
	pids, err := e.pidsForDataDir(dataDir)
	if err != nil {
		return err
	}
	args := []string{"-l", "JavaScript", "-e", activateJS}
	for _, pid := range pids {
		args = append(args, strconv.Itoa(pid))
	}
	return exec.Command("osascript", args...).Run()
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
