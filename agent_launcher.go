//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
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
	// We launch the agent into the *active console session* (the logged-in user's desktop).
	sessionID, err := activeConsoleSessionID()
	if err != nil {
		return nil, err
	}

	// This is the path to the current executable. The spawned agent is just another
	// instance of this binary, but running inside the user session.
	exePath, err := executablePath()
	if err != nil {
		return nil, err
	}

	// CreateProcessAsUser requires a primary token. WTSQueryUserToken provides an
	// impersonation token, so we duplicate it into a primary token.
	primary, err := primaryTokenForSession(sessionID)
	if err != nil {
		return nil, err
	}
	defer primary.Close() //nolint:errcheck

	// Use the user's environment variables so the agent behaves like a normal app
	// (e.g., user profile paths, PATH, etc.).
	envBlock, err := userEnvBlock(primary)
	if err != nil {
		return nil, err
	}

	// Build the command line for the agent instance.
	cmdPtr, appPtr, err := buildAgentCommandLine(exePath, configPath, logPath)
	if err != nil {
		return nil, err
	}

	// Ensure the agent is attached to the interactive desktop (required to register
	// global hotkeys).
	si, err := startupInfoForInteractiveDesktop()
	if err != nil {
		return nil, err
	}

	pi, err := createAgentProcess(primary, appPtr, cmdPtr, envBlock, &si)
	if err != nil {
		return nil, err
	}
	return pi, nil
}

func activeConsoleSessionID() (uint32, error) {
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xFFFFFFFF {
		return 0, errors.New("no active console session")
	}
	return sessionID, nil
}

func executablePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("executable: %w", err)
	}
	return exePath, nil
}

func primaryTokenForSession(sessionID uint32) (windows.Token, error) {
	var token windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &token); err != nil {
		return 0, fmt.Errorf("WTSQueryUserToken(session=%d): %w", sessionID, err)
	}
	defer token.Close() //nolint:errcheck

	// WTSQueryUserToken yields an impersonation token; CreateProcessAsUser needs a primary.
	var primary windows.Token
	if err := windows.DuplicateTokenEx(
		token,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityIdentification,
		windows.TokenPrimary,
		&primary,
	); err != nil {
		return 0, fmt.Errorf("DuplicateTokenEx: %w", err)
	}

	return primary, nil
}

func userEnvBlock(token windows.Token) ([]uint16, error) {
	env, err := token.Environ(false)
	if err != nil {
		return nil, fmt.Errorf("token environ: %w", err)
	}
	sort.Strings(env)

	envBlock, err := encodeEnvBlock(env)
	if err != nil {
		return nil, fmt.Errorf("encode env: %w", err)
	}
	return envBlock, nil
}

func buildAgentCommandLine(exePath, configPath, logPath string) (cmdPtr, appPtr *uint16, err error) {
	cmdLine := syscall.EscapeArg(exePath)
	cmdLineArgs := []string{"--config", configPath}
	if logPath != "" {
		cmdLineArgs = append(cmdLineArgs, "--log", logPath)
	}
	for _, a := range cmdLineArgs {
		cmdLine += " " + syscall.EscapeArg(a)
	}

	cmdPtr, err = syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, nil, fmt.Errorf("command line utf16: %w", err)
	}
	appPtr, err = syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return nil, nil, fmt.Errorf("app utf16: %w", err)
	}

	return cmdPtr, appPtr, nil
}

func startupInfoForInteractiveDesktop() (windows.StartupInfo, error) {
	// The default interactive desktop for a user session.
	desktopPtr, err := syscall.UTF16PtrFromString("winsta0\\default")
	if err != nil {
		return windows.StartupInfo{}, fmt.Errorf("desktop utf16: %w", err)
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = desktopPtr
	return si, nil
}

func createAgentProcess(primary windows.Token, appPtr, cmdPtr *uint16, envBlock []uint16, si *windows.StartupInfo) (*windows.ProcessInformation, error) {
	if si == nil {
		return nil, errors.New("startup info is required")
	}
	if len(envBlock) == 0 {
		return nil, errors.New("environment block is empty")
	}

	var pi windows.ProcessInformation
	// CREATE_UNICODE_ENVIRONMENT matches our UTF-16 environment block.
	// CREATE_NO_WINDOW keeps the agent window-less (it registers hotkeys via Win32 APIs).
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
		si,
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
