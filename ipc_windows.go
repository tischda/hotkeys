//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

const hotkeysIPCPipeEnvVar = "HOTKEYS_IPC_PIPE"

var (
	ipcMu     sync.Mutex
	ipcHandle windows.Handle
)

// ipcInitFromEnv connects to a named pipe specified by HOTKEYS_IPC_PIPE.
//
// Parameters: none.
//
// Returns: none.
func ipcInitFromEnv() {
	pipePath := os.Getenv(hotkeysIPCPipeEnvVar)
	if pipePath == "" {
		return
	}

	p, err := syscall.UTF16PtrFromString(pipePath)
	if err != nil {
		log.Printf("ipc: invalid pipe path: %v", err)
		return
	}

	h, err := windows.CreateFile(
		p,
		windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		log.Printf("ipc: connect %q: %v", pipePath, err)
		return
	}

	ipcMu.Lock()
	ipcHandle = h
	ipcMu.Unlock()

	log.Printf("ipc: connected to %s", pipePath)
}

// ipcSendf sends a formatted message to the service IPC pipe if connected.
//
// Parameters:
//   - format: fmt.Sprintf format string.
//   - args: Arguments referenced by format.
//
// Returns: none.
func ipcSendf(format string, args ...any) {
	ipcMu.Lock()
	h := ipcHandle
	ipcMu.Unlock()
	if h == 0 {
		return
	}

	msg := fmt.Sprintf(format, args...)
	b := append([]byte(msg), '\n')

	var n uint32
	if err := windows.WriteFile(h, b, &n, nil); err != nil {
		log.Printf("ipc: write: %v", err)
	}
}

// ipcClose closes the IPC pipe connection, if any.
//
// Parameters: none.
//
// Returns: none.
func ipcClose() {
	ipcMu.Lock()
	h := ipcHandle
	ipcHandle = 0
	ipcMu.Unlock()

	if h == 0 {
		return
	}
	_ = windows.CloseHandle(h)
}
