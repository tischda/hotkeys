//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Services cannot register global hotkeys like Alt+D because they lack access to user
// input devices and desktops in user sessions (Session 1+).
// Error 1459 (ERROR_REQUIRES_INTERACTIVE_WINDOW_STATION) confirms the issue.

// Solutions
// Launch a separate user-mode app: From the service, use CreateProcessAsUser to start
// a helper executable in the active user session (query tokens via WTSEnumerateSessions/Ex),
// where it registers the hotkey and communicates back via IPC (named pipes or shared memory)
//
//
//
//

// launchAgentInActiveSession starts a helper instance of this executable in the active
// interactive user session so it can register hotkeys on the user's desktop.
//
// Parameters:
//   - configPath: Path to the config file to pass through to the agent.
//   - logPath: Optional log path to pass through to the agent.
//
// Returns:
//   - *windows.ProcessInformation: Handles and IDs for the created process.
//   - error: Non-nil if no interactive session is available or process creation fails.
func launchAgentInActiveSession(configPath, logPath string) (*windows.ProcessInformation, error) {
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		return nil, errors.New("no active console session")
	}

	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("executable: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return nil, fmt.Errorf("abs executable: %w", err)
	}

	var token windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &token); err != nil {
		return nil, fmt.Errorf("WTSQueryUserToken(session=%d): %w", sessionID, err)
	}
	defer token.Close()

	var primary windows.Token
	if err := windows.DuplicateTokenEx(
		token,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityIdentification,
		windows.TokenPrimary,
		&primary,
	); err != nil {
		return nil, fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer primary.Close()

	env, err := primary.Environ(false)
	if err != nil {
		return nil, fmt.Errorf("token environ: %w", err)
	}
	sort.Strings(env)
	envBlock, err := encodeEnvBlock(env)
	if err != nil {
		return nil, fmt.Errorf("encode env: %w", err)
	}

	cmdLine := syscall.EscapeArg(exePath)
	cmdLineArgs := []string{"--config", configPath}
	if logPath != "" {
		cmdLineArgs = append(cmdLineArgs, "--log", logPath)
	}
	for _, a := range cmdLineArgs {
		cmdLine += " " + syscall.EscapeArg(a)
	}
	cmdPtr, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, fmt.Errorf("command line utf16: %w", err)
	}
	appPtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return nil, fmt.Errorf("app utf16: %w", err)
	}

	desktopPtr, err := syscall.UTF16PtrFromString("winsta0\\default")
	if err != nil {
		return nil, fmt.Errorf("desktop utf16: %w", err)
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = desktopPtr

	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW)
	if err := windows.CreateProcessAsUser(
		primary,
		appPtr,
		cmdPtr,
		nil,
		nil,
		false,
		flags,
		&envBlock[0],
		nil,
		&si,
		&pi,
	); err != nil {
		return nil, fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	return &pi, nil
}

func encodeEnvBlock(env []string) ([]uint16, error) {
	// Environment block is a sequence of UTF-16 strings, each terminated by NUL,
	// and the entire block terminated by an extra NUL.
	block := make([]uint16, 0, 1024)
	for _, e := range env {
		if e == "" {
			continue
		}
		if strings.IndexByte(e, 0) != -1 {
			return nil, fmt.Errorf("env contains NUL")
		}
		u, err := syscall.UTF16FromString(e)
		if err != nil {
			return nil, err
		}
		block = append(block, u...)
	}
	block = append(block, 0)
	return block, nil
}
