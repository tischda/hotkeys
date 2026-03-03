package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// logger is safe to use before setupLogging runs (e.g., in tests).
// setupLogging overwrites this with the desired output and formatting.
var logger = log.New(io.Discard, "", 0)

// Setup file logger for user mode.
// If --log is empty, stdout is used.
// If --log is set, it is treated as a file path.
func setupLogging(cfg *Config) (*os.File, error) {
	var logFile *os.File
	var f *os.File
	var err error

	if cfg.logPath == "" {
		f = os.Stdout
		log.SetOutput(f)
		log.SetFlags(0)
		logger = log.New(f, "", 0)
	} else {
		logPath := cfg.logPath

		// Ensure parent directory exists for file logging.
		if dir := filepath.Dir(logPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}

		f, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		logFile = f // this one needs to be closed later (not stdout)
		log.SetOutput(f)
		logger = log.New(f, "", log.LstdFlags)
		logger.Printf("-------------------- PROCESS START [user pid=%d] --------------------", os.Getpid())
	}
	return logFile, nil
}
