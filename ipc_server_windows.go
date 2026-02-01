//go:build windows

package main

import (
	"fmt"
	"math/rand/v2"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	pipeBufferSize = 16 * 1024
)

// startIPCServer creates a best-effort named pipe server for receiving messages from the agent.
//
// Parameters: none.
//
// Returns:
//   - string: The full pipe path (e.g. \\.\pipe\hotkeys-123) to pass to the agent.
//   - func(): A stop function that closes the pipe.
//   - error: Non-nil if the pipe cannot be created.
func startIPCServer() (string, func(), error) {
	name := fmt.Sprintf("hotkeys-%d-%d", time.Now().UnixNano(), rand.Uint32())
	pipePath := `\\.\pipe\` + name

	p, err := syscall.UTF16PtrFromString(pipePath)
	if err != nil {
		return "", nil, err
	}

	h, err := windows.CreateNamedPipe(
		p,
		windows.PIPE_ACCESS_INBOUND,
		windows.PIPE_TYPE_MESSAGE|windows.PIPE_READMODE_MESSAGE|windows.PIPE_WAIT,
		1,
		pipeBufferSize,
		pipeBufferSize,
		0,
		nil,
	)
	if err != nil {
		return "", nil, err
	}

	stop := func() {
		_ = windows.CloseHandle(h)
	}

	go func() {
		defer stop()

		if err := windows.ConnectNamedPipe(h, nil); err != nil {
			// If the client connected between CreateNamedPipe and ConnectNamedPipe.
			if err != windows.ERROR_PIPE_CONNECTED {
				logger.Printf("ipc: connect: %v", err)
				return
			}
		}

		buf := make([]byte, pipeBufferSize)
		for {
			var n uint32
			err := windows.ReadFile(h, buf, &n, nil)
			if err != nil {
				return
			}
			if n == 0 {
				continue
			}
			logger.Printf("agent: %s", string(buf[:n]))
		}
	}()

	return pipePath, stop, nil
}
