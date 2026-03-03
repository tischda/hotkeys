package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

// default config file path containing the hotkey bindings
const DEFAULT_CONFIG_FILE = "hotkeys.toml"
const DEFAULT_CONFIG_PATH = `%USERPROFILE%\.config\` + DEFAULT_CONFIG_FILE

// takes precedence over DEFAULT_CONFIG_PATH above
const HOTKEYS_CONFIG_HOME_VAR = "HOTKEYS_CONFIG_HOME"

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
	flag.StringVar(&cfg.logPath, "log", "", "specify log output file path")
	flag.BoolVar(&cfg.help, "?", false, "")
	flag.BoolVar(&cfg.help, "help", false, "displays this help message")
	flag.BoolVar(&cfg.version, "v", false, "")
	flag.BoolVar(&cfg.version, "version", false, "print version and exit")
	return cfg
}

// full path to the config file (including filename)
var configPath string

// Hotkey internal representation
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
		fmt.Fprintln(os.Stderr, "Usage: "+name+` [COMMAND] [OPTIONS]

Starts a hotkey daemon that binds hotkeys such as CTRL+A to an action. The
bindings are defined in a TOML config file (hot-reload supported).

COMMANDS:

  install [--force]  creates/updates a Task Scheduler logon entry
  remove             removes the Task Scheduler logon entry
  status             shows Task Scheduler state (scheduled/running)

OPTIONS:

  -c, --config path
        specify config file path (default '`+DEFAULT_CONFIG_PATH+`')
  -l, --log path
        specify log output file path (default stdout)
  -?, --help
        display this help message
  -v, --version
        print version and exit`)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			subFlags := flag.NewFlagSet("install", flag.ExitOnError)
			subFlags.StringVar(&cfg.configPath, "config", DEFAULT_CONFIG_PATH, "")
			subFlags.StringVar(&cfg.logPath, "log", "", "")
			force := false
			subFlags.BoolVar(&force, "force", false, "")
			if err := subFlags.Parse(os.Args[2:]); err != nil {
				os.Exit(1)
			}

			installConfigPath := os.Getenv(HOTKEYS_CONFIG_HOME_VAR)
			if installConfigPath != "" {
				installConfigPath = filepath.Join(installConfigPath, DEFAULT_CONFIG_FILE)
			} else {
				installConfigPath = expandVariable(cfg.configPath)
			}

			if err := installStartupTask(defaultTaskName, installConfigPath, cfg.logPath, force); err != nil {
				log.Fatalf("install failed: %v", err)
			}
			log.Printf("Task '%s' installed.", defaultTaskName)
			return

		case "remove":
			subFlags := flag.NewFlagSet("remove", flag.ExitOnError)
			if err := subFlags.Parse(os.Args[2:]); err != nil {
				os.Exit(1)
			}

			if err := removeStartupTask(defaultTaskName); err != nil {
				log.Fatalf("remove failed: %v", err)
			}
			log.Printf("Task '%s' removed.", defaultTaskName)
			return

		case "status":
			subFlags := flag.NewFlagSet("status", flag.ExitOnError)
			if err := subFlags.Parse(os.Args[2:]); err != nil {
				os.Exit(1)
			}

			status, err := getStartupTaskStatus(defaultTaskName)
			if err != nil {
				log.Fatalf("status failed: %v", err)
			}

			if !status.Scheduled {
				log.Printf("Task '%s' scheduled=no", defaultTaskName)
				return
			}
			log.Printf("Task '%s' scheduled=yes status=%s", defaultTaskName, status.Status)
			return
		}
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

	releaseInstanceLock, err := acquireSingleInstanceLock()
	if err != nil {
		if errors.Is(err, errAlreadyRunning) {
			log.Println("hotkeys is already running")
			return
		}
		log.Fatalf("failed to acquire single-instance lock: %v", err)
	}
	defer releaseInstanceLock()

	// Determine config path
	configPath = os.Getenv(HOTKEYS_CONFIG_HOME_VAR)
	if configPath != "" {
		configPath = filepath.Join(configPath, DEFAULT_CONFIG_FILE)
	} else {
		configPath = expandVariable(cfg.configPath)
	}

	// Setup logging
	logFile, err := setupLogging(cfg)
	if err != nil {
		log.Fatalf("Failed to setup logging: %v", err)
	}
	defer func() {
		if logFile != nil {
			logger.Println("Closing log file")
			logFile.Close() //nolint:errcheck
		}
	}()
	minimizeConsoleWindow()
	runServer()
}

// runServer is your actual server logic.
func runServer() {
	logger.Printf("Server starting ... ")
	logger.Printf("%s %s, built on %s (commit: %s)", name, version, date, commit)

	runtime.LockOSThread()

	var err error
	hwnd, err := createHiddenWindow("HotkeyWindow")
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
