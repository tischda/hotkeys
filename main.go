package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
)

// https://goreleaser.com/cookbooks/using-main.version/
var (
	name    string
	version string
	date    string
	commit  string
)

// flags
type Config struct {
	help    bool
	version bool
	path    string
}

// default config file path containing the hotkey bindings
const DEFAULT_CONFIG_PATH = "%USERPROFILE%\\.config\\hotkeys.toml"

// takes precedence over DEFAULT_CONFIG_PATH above
const HOTKEYS_CONFIG_HOME_VAR = "HOTKEYS_CONFIG_HOME"

func initFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.path, "f", DEFAULT_CONFIG_PATH, "")
	flag.StringVar(&cfg.path, "file", DEFAULT_CONFIG_PATH, "specify config file path")
	flag.BoolVar(&cfg.help, "?", false, "")
	flag.BoolVar(&cfg.help, "help", false, "displays this help message")
	flag.BoolVar(&cfg.version, "v", false, "")
	flag.BoolVar(&cfg.version, "version", false, "print version and exit")
	return cfg
}

type ConfigFile struct {
	Keybindings KeybindingsConfig `toml:"keybindings"`
}

type KeybindingsConfig struct {
	Bindings []Binding `toml:"bindings"`
}

type Binding struct {
	Modifiers string   `toml:"modifiers"`
	Key       string   `toml:"key"`
	Action    []string `toml:"action"`
}

type Hotkey struct {
	Id        uint32   // Unique identifier for the hotkey required by RegisterHotKey
	Modifiers uint32   // Translated Modifier keys (Alt, Ctrl, Shift, Win)
	KeyCode   uint16   // Translated Virtual-Key code
	KeyString string   // Original key string for reference
	Action    []string // Command to execute
}

const (
	ModAlt   = 0x0001
	ModCtrl  = 0x0002
	ModShift = 0x0004
	ModSuper = 0x0008
)

// Global slice of hotkeys (global because needed in wndProc)
var hotkeys []Hotkey
var configPath string

// main starts the hotkey daemon, loads config, and blocks in the Windows message loop.
func main() {
	log.SetFlags(0)
	cfg := initFlags()
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: "+name+` [OPTIONS]

Starts a hotkey daemon that binds hotkeys such as CTRL+A to an action. The bindings
are defined in a TOML config file (hot-reload supported).

The processes executed by the daemon will inherit the current environment and update
USER and SYSTEM environment variables from the Windows registry.

OPTIONS:

  -f, --file path
        specify config file path (default '%USERPROFILE%\.config\hotkeys.toml')
  -?, --help
        display this help message
  -v, --version
        print version and exit`)
	}
	flag.Parse()

	if flag.Arg(0) == "version" || cfg.version {
		fmt.Printf("%s %s, built on %s (commit: %s)\n", name, version, date, commit)
		return
	}

	if cfg.help {
		flag.Usage()
		return
	}

	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(1)
	}

	runtime.LockOSThread()

	log.Println("Starting hotkey daemon...")

	var err error
	hwnd, err := createHiddenWindow("HotkeyWindow")
	if err != nil {
		log.Fatalf("Failed to create hidden window: %v", err)
	}
	defer destroyWindow.Call(uintptr(hwnd)) //nolint:errcheck

	// Determine config path
	configPath = os.Getenv(HOTKEYS_CONFIG_HOME_VAR)
	if configPath == "" {
		configPath = expandVariable(cfg.path)
	}

	if err := reloadHotkeys(hwnd); err != nil {
		log.Fatalf("Failed to load config %s: %v", configPath, err)
	}

	// Handle graceful shutdown on Ctrl+C
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	go func() {
		<-interrupt
		log.Println("Exiting...")
		postMessageW.Call(hwnd, WM_APP_QUIT, 0, 0) //nolint:errcheck
	}()

	// Start config file watcher
	watcher, err := startConfigWatcher(hwnd, configPath)
	if err != nil {
		log.Printf("Config watcher disabled: %v", err)
	}
	if watcher != nil {
		defer watcher.Close() //nolint:errcheck
	}

	// Listen for key presses
	messageLoop()

	// Cleanup
	unregisterAll(hwnd)
}
