package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"path/filepath"

	"strings"
)

// default config file path containing the hotkey bindings
const DEFAULT_CONFIG_FILE = "hotkeys.toml"
const DEFAULT_CONFIG_PATH = `%USERPROFILE%\.config\` + DEFAULT_CONFIG_FILE

// takes precedence over DEFAULT_CONFIG_PATH above
const HOTKEYS_CONFIG_HOME_VAR = "HOTKEYS_CONFIG_HOME"

// Task Scheduler name for the hotkeys startup task
const TASK_NAME = "Hotkeys"

// https://goreleaser.com/cookbooks/using-main.version/
var (
	name    string
	version string
	date    string
	commit  string
)

func commandName() string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return filepath.Base(os.Args[0])
}

// flags
type Config struct {
	configPath string
	logPath    string
	background bool
	help       bool
	version    bool
}

func initFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.configPath, "c", DEFAULT_CONFIG_PATH, "")
	flag.StringVar(&cfg.configPath, "config", DEFAULT_CONFIG_PATH, "specify config file path")
	flag.StringVar(&cfg.logPath, "l", "", "")
	flag.StringVar(&cfg.logPath, "log", "", "specify log output file path")
	flag.BoolVar(&cfg.background, "b", false, "")
	flag.BoolVar(&cfg.background, "background", false, "start in background without a console window")
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

Starts a hotkey daemon that binds hotkeys such as ALT+ENTER to an action.
The bindings are defined in a TOML config file (hot-reload supported).

COMMANDS:

  install [--force]  creates/updates a Task Scheduler entry
  remove             removes the Task Scheduler entry
  start              starts the scheduled task
  stop               stops the running hotkeys process
  status             shows the scheduled task and process states

OPTIONS:

  -c, --config path
        specify config file path (default '`+DEFAULT_CONFIG_PATH+`')
  -l, --log path
        specify log output file path (default stdout)
  -b, --background
        start in background without a console window
  -?, --help
        display this help message
  -v, --version
        print version and exit`)
	}

	// Determine config path
	configPath = os.Getenv(HOTKEYS_CONFIG_HOME_VAR)
	if configPath != "" {
		configPath = filepath.Join(configPath, DEFAULT_CONFIG_FILE)
	} else {
		configPath = expandVariable(cfg.configPath)
	}

	// Handle Task Scheduler subcommands if present
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

			if err := installStartupTask(TASK_NAME, configPath, cfg.logPath, force); err != nil {
				log.Fatalf("install failed: %v", err)
			}
			log.Printf("Task '%s' installed.", TASK_NAME)
			return

		case "remove":
			if err := removeStartupTask(TASK_NAME); err != nil {
				log.Fatalf("remove failed: %v", err)
			}
			log.Printf("Task '%s' removed.", TASK_NAME)
			return

		case "start":
			status := validateTaskStatus()
			if status.ProcessRunning {
				log.Printf("Hotkeys process already running (pid=%d)", status.ProcessID)
				return
			}
			if err := startStartupTask(TASK_NAME); err != nil {
				log.Fatalf("start failed: %v", err)
			}
			log.Printf("Task '%s' started.", TASK_NAME)
			return

		case "stop":
			status := validateTaskStatus()
			if !status.ProcessRunning {
				log.Printf("Hotkeys process already stopped.")
				return
			}
			if err := stopProcessGracefully(status.ProcessID); err != nil {
				log.Fatalf("stop failed: %v", err)
			}
			log.Printf("Stop signal sent to process pid=%d", status.ProcessID)
			return

		case "status":
			status, err := getStartupTaskStatus(TASK_NAME)
			if err != nil {
				log.Fatalf("status failed: %v", err)
			}

			if !status.Scheduled {
				log.Printf("Task '%s' scheduled=no", TASK_NAME)
				return
			}
			if status.ProcessRunning {
				log.Printf("Task '%s' scheduled=yes status=%s, process=Running (pid=%d)", TASK_NAME, status.Status, status.ProcessID)
				return
			}

			log.Printf("Task '%s' scheduled=yes status=%s, process=Stopped", TASK_NAME, status.Status)
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

	// When running with --background, the process is re-executed in a detached state without
	// a console window. In that case, the parent process should exit immediately and not run
	// the server logic below. The detached child process will continue to run the server.
	detached, err := startDetached(cfg.background)
	if err != nil {
		log.Fatalf("failed to start in background: %v", err)
	}
	if detached {
		return
	}

	// Ensure only one instance is running
	releaseInstanceLock, err := acquireSingleInstanceLock()
	if err != nil {
		if errors.Is(err, errAlreadyRunning) {
			log.Println("hotkeys is already running")
			return
		}
		log.Fatalf("failed to acquire single-instance lock: %v", err)
	}
	defer releaseInstanceLock()

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
	runServer()
}
