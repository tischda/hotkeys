//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartConfigWatcher_SymlinkTargetChangeTriggersReload(t *testing.T) {
	tempRoot := t.TempDir()
	linkDir := filepath.Join(tempRoot, "link")
	targetDir := filepath.Join(tempRoot, "target")

	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir linkDir: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir targetDir: %v", err)
	}

	targetPath := filepath.Join(targetDir, "hotkeys.toml")
	linkPath := filepath.Join(linkDir, "hotkeys.toml")

	if err := os.WriteFile(targetPath, []byte("[keybindings]\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := os.Symlink(targetPath, linkPath); err != nil {
		// Windows symlink creation can require admin rights or Developer Mode.
		t.Skipf("symlink not available on this system: %v", err)
	}

	reloadCh := make(chan struct{}, 10)
	watcher, err := startConfigWatcherWithNotifier(linkPath, func() {
		select {
		case reloadCh <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	t.Cleanup(func() {
		_ = watcher.Close()
	})

	// Give fsnotify a moment to attach.
	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(targetPath, []byte("[keybindings]\n# changed\n"), 0o644); err != nil {
		t.Fatalf("write target changed: %v", err)
	}

	select {
	case <-reloadCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("expected reload signal after modifying symlink target")
	}
}

func TestResolveWatchPaths_ReturnsTargetForSymlink(t *testing.T) {
	tempRoot := t.TempDir()
	linkDir := filepath.Join(tempRoot, "link")
	targetDir := filepath.Join(tempRoot, "target")

	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir linkDir: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir targetDir: %v", err)
	}

	targetPath := filepath.Join(targetDir, "hotkeys.toml")
	linkPath := filepath.Join(linkDir, "hotkeys.toml")

	if err := os.WriteFile(targetPath, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("symlink not available on this system: %v", err)
	}

	gotLink, gotTarget := resolveWatchPaths(linkPath)
	if gotLink == "" {
		t.Fatalf("expected non-empty link path")
	}
	if gotTarget == "" {
		t.Fatalf("expected non-empty target path for symlink")
	}
}
