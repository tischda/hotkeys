package main

import (
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

// shouldReloadConfig reports whether an fsnotify event warrants a config reload.
//
// Parameters:
//   - configPath: Cleaned absolute path to the config file.
//   - configBase: Base filename of the config file.
//   - event: Filesystem event to evaluate.
//
// Returns:
//   - bool: True if the event should trigger a reload.
func shouldReloadConfig(configPath, configBase string, event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
		return false
	}
	name := filepath.Clean(event.Name)
	if name == configPath {
		return true
	}
	// Some editors write via temp + rename, resulting in partial paths.
	if filepath.Base(name) == configBase {
		return true
	}
	return false
}

// reloadHotkeys unregisters all hotkeys,loads and register the current config.
//
// Parameters:
//   - hwnd: Handle to the message-only window whose hotkeys are registered.
//
// Returns:
//   - error: Non-nil if the config cannot be loaded.
func reloadHotkeys(hwnd uintptr) error {

	// 1. Start from a clean state
	unregisterAll(hwnd)

	// 2. Load and register hotkeys from config
	newHotkeys, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	hotkeys = newHotkeys

	// 3. Register all hotkeys
	registerAll(hwnd)

	logger.Printf("Loaded and registered %d bindings from %s", len(hotkeys), configPath)
	return nil
}

// loadConfig reads a TOML config file and converts it to a list of hotkeys.
//
// Parameters:
//   - path: Path to the TOML config file.
//
// Returns:
//   - []Hotkey: Parsed hotkeys in registration order.
//   - error: Non-nil if the file cannot be decoded.
func loadConfig(path string) ([]Hotkey, error) {
	var config ConfigFile
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("decode %w", err)
	}
	var keyList []Hotkey
	var nextID uint32 = 1

	for _, binding := range config.Keybindings.Bindings {
		hk := parseHotkey(binding.Modifiers, binding.Key)
		if hk.KeyCode == '?' {
			logger.Printf("Skipping invalid hotkey: %s + %s", binding.Modifiers, binding.Key)
			continue
		}
		keyList = append(keyList, Hotkey{
			Id:        nextID,
			Modifiers: hk.Modifiers,
			KeyCode:   hk.KeyCode,
			KeyString: binding.Modifiers + "+" + binding.Key,
			Action:    binding.Action,
		})
		nextID++
	}
	return keyList, nil
}
