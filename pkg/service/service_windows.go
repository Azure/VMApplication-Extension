package service

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	// Service start types.
	StartManual    = windows.SERVICE_DEMAND_START // the service must be started manually
	StartAutomatic = windows.SERVICE_AUTO_START   // the service will start by itself whenever the computer reboots
	StartDisabled  = windows.SERVICE_DISABLED     // the service cannot be started

	// The severity of the error, and action taken,
	// if this service fails to start.
	ErrorCritical = windows.SERVICE_ERROR_CRITICAL
	ErrorIgnore   = windows.SERVICE_ERROR_IGNORE
	ErrorNormal   = windows.SERVICE_ERROR_NORMAL
	ErrorSevere   = windows.SERVICE_ERROR_SEVERE
)

type WindowsServiceManager struct{}

type WindowsService struct {
	Config *ServiceConfig
}

func (WindowsServiceManager) String() string {
	return "windows-service"
}

func (WindowsServiceManager) DetectIsAvailable() bool {
	return true
}

func (WindowsServiceManager) New(c *ServiceConfig) (Service, error) {
	service := &WindowsService{
		Config: c,
	}
	return service, nil
}

func (ws *WindowsService) Install() error {
	exepath, err := filepath.Abs(ws.Config.Executable)
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Config.Name)
	if err == nil {
		s.Close()
		return fmt.Errorf("Service %s already exists", ws.Config.Name)
	}

	s, err = m.CreateService(ws.Config.Name, exepath, mgr.Config{
		StartType:        windows.SERVICE_AUTO_START,        // Automatically load and run the service on bootup
		ServiceType:      windows.SERVICE_WIN32_OWN_PROCESS, // Service should be run as a stand-alone process
		ErrorControl:     windows.SERVICE_ERROR_NORMAL,      // If service fails to startup upon boot, produce a warning but let bootup continue
		DelayedAutoStart: true,                              // Start service with a delay after other auto-start services are started
		DisplayName:      ws.Config.DisplayName,
	})

	if err != nil {
		return err
	}
	defer s.Close()

	return nil
}

func (ws *WindowsService) Uninstall() error {
	return nil
}

func (ws *WindowsService) Start() error {
	return nil
}

func (ws *WindowsService) Stop() error {
	return nil
}

func (ws *WindowsService) Restart() error {
	return nil
}

func (ws *WindowsService) IsActive() bool {
	return false
}
