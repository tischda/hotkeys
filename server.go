package main

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

// runServer is your actual server logic.
func runServer() {
	logger.Printf("Server starting ... ")
	logger.Printf("%s %s, built on %s (commit: %s)", name, version, date, commit)

	runtime.LockOSThread()

	var err error
	hwnd, err := createHiddenWindow(hotkeyWindowClassName)
	if err != nil {
		logger.Fatalf("Failed to create hidden window: %v", err)
	}
	defer destroyWindow.Call(uintptr(hwnd)) //nolint:errcheck

	// Initial config load
	if err := reloadHotkeys(hwnd); err != nil {
		logger.Fatalf("Failed to load config %s: %v", configPath, err)
	}

	// Handle graceful shutdown on console/task stop signals.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)
	go func() {
		sig := <-interrupt
		logger.Printf("Exiting on signal: %v", sig)
		postMessageW.Call(hwnd, WM_APP_QUIT, 0, 0) //nolint:errcheck
	}()

	// Start config file watcher
	watcher, err := startConfigWatcher(hwnd, configPath)
	if err != nil {
		logger.Printf("Config watcher disabled: %v", err)
	}
	if watcher != nil {
		defer watcher.Close() //nolint:errcheck
	}

	// Listen for key presses
	messageLoop()

	// Cleanup
	unregisterAll(hwnd)
}
