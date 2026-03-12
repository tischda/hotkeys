//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// startDetached starts a detached child process when background mode is requested.
//
// Returns true when a detached child has been started and the current process should exit.
func startDetached(background bool) (bool, error) {
	if !background {
		return false, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolve executable path: %w", err)
	}

	childArgs := make([]string, 0, len(os.Args)-1)
	for _, arg := range os.Args[1:] {
		if arg == "-b" || arg == "--background" {
			continue
		}
		childArgs = append(childArgs, arg)
	}

	cmd := exec.Command(exePath, childArgs...)
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS,
	}

	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start detached child: %w", err)
	}

	return true, nil
}

// executeCommand starts a new process specified by cmd in a detached state on Windows.
// The new process will not be attached to the current console and will run independently.
//
// The process will also inherit a new set of user and system environment variables.
//
// Parameters:
//   - cmd: The executable to run and its arguments as a slice of strings.

// Returns:
//   - int: The process ID of the started process.
//   - error: Non-nil if process creation or startup fails.
//
// The function returns the process ID of the new process or an error if the process creation fails.
func executeCommand(cmd []string) (int, error) {
	if len(cmd) == 0 {
		return 0, errors.New("command array is empty")
	}
	c := exec.Command(cmd[0], cmd[1:]...)

	c.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS,
	}

	// prepare environment for process
	env, err := getUserAndSystemEnv()
	if err != nil {
		return 0, fmt.Errorf("failed to get environment: %w", err)
	}
	c.Env = env

	// start process
	err = c.Start()
	if err != nil {
		return 0, fmt.Errorf("failed to start command %v : %w", cmd, err)
	}
	return c.Process.Pid, nil
}
