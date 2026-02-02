package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	SERVICE_NAME        = "Hotkeys"
	SERVICE_DISPLAYNAME = "Hotkeys Service"
	SERVICE_DESCRIPTION = "Binds Windows hotkeys to specific actions"
)

// service struct implementing svc.Handler
type myService struct {
	config string
	log    string
}

// Execute is called by the Windows service manager.
func (m *myService) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	s <- svc.Status{State: svc.StartPending}

	// Log to file or stdout
	logger.Printf("Execute with config=%s, log=%s", m.config, m.log)

	pi, err := launchAgentInActiveSession(m.config, m.log)
	if err != nil {
		logger.Printf("Failed to launch agent in active session: %v", err)
		s <- svc.Status{State: svc.Stopped}
		return false, 1
	}
	defer func() {
		// Best-effort cleanup.
		_ = windows.CloseHandle(pi.Thread)
		_ = windows.CloseHandle(pi.Process)
	}()

	s <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			logger.Println("Service received stop signal")
			_ = windows.TerminateProcess(pi.Process, 0)
			break loop
		default:
		}
	}

	s <- svc.Status{State: svc.StopPending}
	// wait for server shutdown if needed
	time.Sleep(500 * time.Millisecond)
	s <- svc.Status{State: svc.Stopped}
	logger.Println("Service stopped")
	return false, 0
}

// installService installs the current executable as a Windows service
// and sets the config/log arguments into the service configuration.
func installService(cfg, logf string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot get executable path: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("cannot get absolute path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("cannot connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Check if service already exists
	if s, err := m.OpenService(SERVICE_NAME); err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", SERVICE_NAME)
	}

	config := mgr.Config{
		DisplayName: SERVICE_DISPLAYNAME,
		Description: SERVICE_DESCRIPTION,
		StartType:   mgr.StartAutomatic,
	}

	// args here become part of the service command line when started:
	// hotkeys.exe --config cfg --log logf
	var s *mgr.Service
	if logf == "" {
		s, err = m.CreateService(SERVICE_NAME, exePath, config, "--config", cfg)
	} else {
		s, err = m.CreateService(SERVICE_NAME, exePath, config,
			"--config", cfg,
			"--log", logf,
		)
	}
	if err != nil {
		return fmt.Errorf("cannot create service: %w", err)
	}
	defer s.Close()
	return nil
}

// removeService removes the Windows service.
func removeService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("cannot connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(SERVICE_NAME)
	if err != nil {
		return fmt.Errorf("service %s is not installed: %w", SERVICE_NAME, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	return nil
}

// runService starts the Windows service handler.
func runService(logf string) {
	var err error

	ms := &myService{
		config: configPath,
		log:    logf,
	}

	err = svc.Run(SERVICE_NAME, ms)
	if err != nil {
		logger.Printf("svc.Run failed: %v", err)
		return
	}
	logger.Println("Service stopped normally")
}
