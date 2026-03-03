package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

const (
	serviceLogFileName = "hotkeys-service.log"
	agentLogFileName   = "hotkeys-agent.log"
)

// logger is safe to use before setupLogging runs (e.g., in tests).
// setupLogging overwrites this with the desired output and formatting.
var logger = log.New(io.Discard, "", 0)

func logFileNameForMode(mode string) string {
	if mode == "service" {
		return serviceLogFileName
	}
	return agentLogFileName
}

// Setup file logger that works in BOTH service and console mode.
// If --log is empty, stdout is used.
// If --log is set, it is treated as a directory and the filename depends on mode.
func setupLogging(cfg *Config, mode string) (*os.File, error) {
	var logFile *os.File
	var f *os.File
	var err error

	if cfg.logPath == "" {
		f = os.Stdout
		log.SetOutput(f)
		log.SetFlags(0)
		logger = log.New(f, "", 0)
	} else {
		logPath := filepath.Join(cfg.logPath, logFileNameForMode(mode))

		// Ensure directory exists for file logging
		if err := os.MkdirAll(cfg.logPath, 0755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", cfg.logPath, err)
		}

		f, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		logFile = f // this one needs to be closed later (not stdout)
		log.SetOutput(f)
		logger = log.New(f, "["+SERVICE_NAME+"] ", log.LstdFlags)
		logger.Printf("-------------------- PROCESS START [%s pid=%d] --------------------", mode, os.Getpid())
		logger.Printf("%s %s, built on %s (commit: %s)", name, version, date, commit)
	}
	return logFile, nil
}
