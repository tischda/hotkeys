package main

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

func resolveWatchPaths(configPath string) (linkPath, targetPath string) {
	// Normalize to an absolute path so event matching is consistent.
	linkPath = filepath.Clean(configPath)
	if abs, err := filepath.Abs(linkPath); err == nil {
		linkPath = abs
	}

	// If configPath is a symlink, resolve its target so we can watch the correct
	// directory for changes. If we can't resolve (missing file, no symlink, etc.),
	// we fall back to watching only the link's directory.
	resolved, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		return linkPath, ""
	}
	resolved = filepath.Clean(resolved)
	if abs, err := filepath.Abs(resolved); err == nil {
		resolved = abs
	}
	if resolved == linkPath {
		return linkPath, ""
	}
	return linkPath, resolved
}

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
	return startConfigWatcherWithNotifier(configPath, func() {
		postMessageW.Call(hwnd, WM_APP_RELOAD, 0, 0) //nolint:errcheck
	})
}

func startConfigWatcherWithNotifier(configPath string, onReload func()) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if onReload == nil {
		onReload = func() {}
	}

	// Watching a directory is more reliable on Windows than watching a single file.
	//
	// If configPath is a symlink, edits to the *target* may happen in a different
	// directory, so we also watch the target's directory when we can resolve it.
	linkPath, targetPath := resolveWatchPaths(configPath)

	linkDir := filepath.Dir(linkPath)
	if err := watcher.Add(linkDir); err != nil {
		watcher.Close() //nolint:errcheck
		return nil, err
	}
	if targetPath != "" {
		targetDir := filepath.Dir(targetPath)
		if targetDir != linkDir {
			if err := watcher.Add(targetDir); err != nil {
				watcher.Close() //nolint:errcheck
				return nil, err
			}
		}
	}

	linkBase := filepath.Base(linkPath)
	targetBase := ""
	if targetPath != "" {
		targetBase = filepath.Base(targetPath)
	}

	go func() {
		var last time.Time
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Trigger reload for either:
				// - changes to the config file itself (symlink path)
				// - changes to the symlink target (when configPath is a symlink)
				if !shouldReloadConfig(linkPath, linkBase, event) && (targetPath == "" || !shouldReloadConfig(targetPath, targetBase, event)) {
					continue
				}
				// Debounce noisy editor save patterns.
				if time.Since(last) < 200*time.Millisecond {
					continue
				}
				last = time.Now()
				logger.Println("Config reload signalled")
				onReload()

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Printf("Config watcher error: %v", err)
			}
		}
	}()
	return watcher, nil
}
