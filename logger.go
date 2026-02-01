package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var logger *log.Logger

// Setup file logger that works in BOTH service and console mode - stdout if --log empty
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
		// Ensure directory exists for file logging
		if err := os.MkdirAll(filepath.Dir(cfg.logPath), 0755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(cfg.logPath), err)
		}

		f, err = os.OpenFile(cfg.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		logFile = f // this one needs to be closed later (not stdout)
		log.SetOutput(f)
		logger = log.New(f, "[MyService] ", log.LstdFlags|log.Lshortfile)
		logger.Println("=== LOG INITIALIZED ===")
	}
	return logFile, nil
}

// TODO: use logger throughout the code instead of log package directly
