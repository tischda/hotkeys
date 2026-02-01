package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"

	"golang.org/x/sys/windows/svc"
)

// default config file path containing the hotkey bindings
const DEFAULT_CONFIG_PATH = `%USERPROFILE%\.config\hotkeys.toml`

// takes precedence over DEFAULT_CONFIG_PATH above
const HOTKEYS_CONFIG_HOME_VAR = "HOTKEYS_CONFIG_HOME"

const (
	SERVICE_NAME        = "Hotkeys"
	SERVICE_DISPLAYNAME = "Hotkeys Service"
	SERVICE_DESCRIPTION = "Binds Windows hotkeys to specific actions"
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
	configPath string
	logPath    string
	help       bool
	version    bool
}

func initFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.configPath, "c", DEFAULT_CONFIG_PATH, "")
	flag.StringVar(&cfg.configPath, "config", DEFAULT_CONFIG_PATH, "specify config file path")
	flag.StringVar(&cfg.logPath, "l", "", "")
	flag.StringVar(&cfg.logPath, "log", "", "specify log output path")
	flag.BoolVar(&cfg.help, "?", false, "")
	flag.BoolVar(&cfg.help, "help", false, "displays this help message")
	flag.BoolVar(&cfg.version, "v", false, "")
	flag.BoolVar(&cfg.version, "version", false, "print version and exit")
	return cfg
}

var configPath string

// Hotkey interanl representation
type Hotkey struct {
	Id        uint32   // Unique identifier for the hotkey required by RegisterHotKey
	Modifiers uint32   // Translated Modifier keys (Alt, Ctrl, Shift, Win)
	KeyCode   uint16   // Translated Virtual-Key code
	KeyString string   // Original key string for reference
	Action    []string // Command to execute
}

var hotkeys []Hotkey // global because needed in wndProc

// Data structures for hotkeys configuration file
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

// main starts the hotkey daemon, loads config, and blocks in the Windows message loop.
func main() {
	log.SetFlags(0)
	cfg := initFlags()
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: "+name+` [COMMANDS] [OPTIONS]

Starts a hotkey daemon that binds hotkeys such as CTRL+A to an action. The
bindings are defined in a TOML config file (hot-reload supported).

The processes executed by the daemon will inherit the current environment
and update USER and SYSTEM environment variables from the Windows registry.

COMMANDS:

  install    installs the application as a Windows service
  remove     removes the Windows service

OPTIONS:

  -c, --config path
        specify config file path (default '`+DEFAULT_CONFIG_PATH+`')
  -l, --log path
        specify log output path (default stdout)
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

	// Re-parse flags after the 'install' subcommand
	if flag.Arg(0) == "install" {
		subFlags := flag.NewFlagSet("install", flag.ExitOnError)
		subFlags.StringVar(&cfg.configPath, "config", DEFAULT_CONFIG_PATH, "")
		subFlags.StringVar(&cfg.logPath, "log", "", "")
		subFlags.Parse(os.Args[2:])
	}

	// Determine config path
	configPath = os.Getenv(HOTKEYS_CONFIG_HOME_VAR)
	if configPath == "" {
		configPath = expandVariable(cfg.configPath)
	}

	// Subcommand logic: install/remove
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := installService(configPath, cfg.logPath); err != nil {
				log.Fatalf("install failed: %v", err)
			}
			log.Println("Service installed.")
			return

		case "remove":
			if err := removeService(); err != nil {
				log.Fatalf("remove failed: %v", err)
			}
			log.Println("Service removed.")
			return
		case "--config", "--log":
			// Handled above
		default:
			log.Fatalf("unknown command: %s", os.Args[1])
		}
	}

	// Setup logging
	logFile, err := setupLogging(cfg)
	if err != nil {
		log.Fatalf("Failed to setup logging: %v", err)
	}
	defer func() {
		if logFile != nil {
			logger.Println("Closing log file")
			logFile.Close()
		}
	}()

	// If we're here, no install/remove: run as service or console.
	isService, err := svc.IsWindowsService()
	if err != nil {
		logger.Fatalf("IsWindowsService: %v", err)
	}

	if isService {
		logger.Printf("Running as service")
		runService(cfg.logPath)
	} else {
		// Fallback for console mode (dev/testing)
		logger.Printf("Running in console mode")
		runServer()
	}

}

// runServer is your actual server logic.
func runServer() {
	logger.Printf("Server starting ... ")

	ipcInitFromEnv()
	defer ipcClose()

	runtime.LockOSThread()

	var err error
	hwnd, err := createHiddenWindow("HotkeyWindow")
	if err != nil {
		log.Fatalf("Failed to create hidden window: %v", err)
	}
	defer destroyWindow.Call(uintptr(hwnd)) //nolint:errcheck

	// Initial config load
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

	// TODO: 1. Implement logging to file (instead of console)
	// TODO: 2. Find a way to stop the deamon gracefully (RPC ?) other than:
	// 				tasklist /FI "IMAGENAME eq hotkeys.exe"
	//				taskkill /f /im hotkeys.exe
	// TODO: 3. Implement a system tray icon with context menu
	// TODO: 4. Investigate running as a Windows Service
	// TODO: 5. Make this a configuration option in the TOML config file
	// Optional: detach from console to avoid showing a console window
	// detachConsole()

	// Listen for key presses
	messageLoop()

	// Cleanup
	unregisterAll(hwnd)
}
