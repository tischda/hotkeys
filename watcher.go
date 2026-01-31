package main

import (
	"github.com/fsnotify/fsnotify"
	"log"
	"path/filepath"
	"time"
)

// startConfigWatcher watches configPath for changes and posts reload messages to hwnd.
//
// Parameters:
//   - hwnd: Handle to the message-only window that receives reload messages.
//   - configPath: Full path to the config file.
//
// Returns:
//   - *fsnotify.Watcher: A watcher the caller should close when done.
//   - error: Non-nil if the watcher cannot be created or the directory cannot be watched.
func startConfigWatcher(hwnd uintptr, configPath string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Watching a directory is more reliable on Windows than watching a single file.
	dir := filepath.Dir(configPath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close() //nolint:errcheck
		return nil, err
	}
	configPath = filepath.Clean(configPath) // full path normalization
	configBase := filepath.Base(configPath) // file name only

	go func() {
		var last time.Time
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !shouldReloadConfig(configPath, configBase, event) {
					continue
				}
				// Debounce noisy editor save patterns.
				if time.Since(last) < 200*time.Millisecond {
					continue
				}
				last = time.Now()
				log.Println("Config reload signalled")
				postMessageW.Call(hwnd, WM_APP_RELOAD, 0, 0)

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Config watcher error: %v", err)
			}
		}
	}()
	return watcher, nil
}
