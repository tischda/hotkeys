package main

import (
	"strings"
)

const (
	ModAlt   = 0x0001
	ModCtrl  = 0x0002
	ModShift = 0x0004
	ModSuper = 0x0008
)

// parseHotkey converts a modifiers+key string pair into a Hotkey with translated key codes.
//
// Parameters:
//   - modifiers: '+' separated modifier names (e.g. "ctrl+shift").
//   - key: Key name (e.g. "a", "f1", "enter").
//
// Returns:
//   - *Hotkey: A Hotkey with Modifiers and KeyCode populated.
func parseHotkey(modifiers, key string) *Hotkey {
	mod := uint32(0)
	for p := range strings.SplitSeq(modifiers, "+") {
		p = strings.TrimSpace(strings.ToLower(p))
		switch p {
		case "alt":
			mod |= ModAlt
		case "ctrl":
			mod |= ModCtrl
		case "shift":
			mod |= ModShift
		case "super", "win":
			mod |= ModSuper
		}
	}
	k := parseKey(strings.TrimSpace(strings.ToLower(key)))
	return &Hotkey{Modifiers: mod, KeyCode: uint16(k)}
}

// parseKey maps a key name to a Windows virtual-key code rune.
//
// Parameters:
//   - s: Normalized key name.
//
// Returns:
//   - rune: A Windows virtual-key code, or '?' if the key is unknown.
func parseKey(s string) rune {
	if len(s) == 1 {
		if s >= "a" && s <= "z" {
			return rune(strings.ToUpper(s)[0])
		}
		if s >= "0" && s <= "9" {
			return rune(s[0])
		}
	}
	keyMap := map[string]rune{
		"enter":  0x0D,
		"return": 0x0D,
		"space":  0x20,
		"tab":    0x09,
		"escape": 0x1B,
		"esc":    0x1B,
		"left":   0x25,
		"up":     0x26,
		"right":  0x27,
		"down":   0x28,
		"f1":     0x70,
		"f2":     0x71,
		"f3":     0x72,
		"f4":     0x73,
		"f5":     0x74,
		"f6":     0x75,
		"f7":     0x76,
		"f8":     0x77,
		"f9":     0x78,
		"f10":    0x79,
		"f11":    0x7A,
		"f12":    0x7B,
	}
	if k, ok := keyMap[s]; ok {
		return k
	}
	return '?'
}
