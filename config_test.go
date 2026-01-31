//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	writeTemp := func(t *testing.T, contents string) string {
		t.Helper()

		dir := t.TempDir()
		path := filepath.Join(dir, "hotkeys.toml")
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write temp config: %v", err)
		}
		return path
	}

	t.Run("parses bindings", func(t *testing.T) {
		t.Parallel()

		path := writeTemp(t, `
[keybindings]
  [[keybindings.bindings]]
  modifiers = "ctrl+alt"
  key = "a"
  action = ["notepad.exe", "/A"]
`)

		hotkeys, err := loadConfig(path)
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if len(hotkeys) != 1 {
			t.Fatalf("expected 1 hotkey, got %d", len(hotkeys))
		}

		hk := hotkeys[0]
		if hk.Id != 1 {
			t.Fatalf("expected Id=1, got %d", hk.Id)
		}
		if hk.Modifiers != (ModCtrl | ModAlt) {
			t.Fatalf("expected Modifiers=%d, got %d", (ModCtrl | ModAlt), hk.Modifiers)
		}
		if hk.KeyCode != uint16('A') {
			t.Fatalf("expected KeyCode=%d, got %d", uint16('A'), hk.KeyCode)
		}
		if hk.KeyString != "ctrl+alt+a" {
			t.Fatalf("expected KeyString=%q, got %q", "ctrl+alt+a", hk.KeyString)
		}
		if len(hk.Action) != 2 || hk.Action[0] != "notepad.exe" || hk.Action[1] != "/A" {
			t.Fatalf("unexpected Action: %#v", hk.Action)
		}
	})

	t.Run("skips invalid bindings", func(t *testing.T) {
		t.Parallel()

		path := writeTemp(t, `
[keybindings]
  [[keybindings.bindings]]
  modifiers = "ctrl"
  key = "definitely-not-a-key"
  action = ["noop"]

  [[keybindings.bindings]]
  modifiers = "shift"
  key = "f1"
  action = ["ok"]
`)

		hotkeys, err := loadConfig(path)
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if len(hotkeys) != 1 {
			t.Fatalf("expected 1 hotkey, got %d", len(hotkeys))
		}
		if hotkeys[0].Id != 1 {
			t.Fatalf("expected Id=1, got %d", hotkeys[0].Id)
		}
		if hotkeys[0].KeyCode != 0x70 {
			t.Fatalf("expected KeyCode=%d, got %d", uint16(0x70), hotkeys[0].KeyCode)
		}
	})

	t.Run("returns error on missing file", func(t *testing.T) {
		t.Parallel()

		_, err := loadConfig(filepath.Join(t.TempDir(), "missing.toml"))
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("wraps decode errors", func(t *testing.T) {
		t.Parallel()

		path := writeTemp(t, `
[keybindings]
  [[keybindings.bindings]]
  modifiers = "ctrl"
  key =
`)

		_, err := loadConfig(path)
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "decode toml:") {
			t.Fatalf("expected wrapped decode error prefix, got %q", err.Error())
		}
	})
}
