package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	ServiceStopTimeout             = 30 * time.Second
	ServiceStopStatusCheckInterval = 500 * time.Millisecond
	WindowsServiceName             = "windows-service"
)

type WindowsServiceManager struct{}

type WindowsService struct {
	Config *ServiceConfig
}

func (WindowsServiceManager) String() string {
	return WindowsServiceName
}

func (WindowsServiceManager) DetectIsAvailable() bool {
	return true
}

func (WindowsServiceManager) New(c *ServiceConfig) (Service, error) {
	if len(c.Name) == 0 {
		return nil, errors.New("Name field within ServiceConfig is required")
	} else if len(c.Executable) == 0 {
		return nil, errors.New("Executable field within ServiceConfig is required")
	}

	service := &WindowsService{
		Config: c,
	}
	return service, nil
}

func (ws *WindowsService) Install() error {
	exePath, err := filepath.Abs(ws.Config.Executable)
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
		return errors.New(fmt.Sprintf("Service %s already exists", ws.Config.Name))
	}

	s, err = m.CreateService(ws.Config.Name, exePath, mgr.Config{
		StartType:        mgr.StartAutomatic, // Automatically load and run the service on bootup
		ErrorControl:     mgr.ErrorNormal,    // If service fails to startup upon boot, produce a warning but let bootup continue
		DelayedAutoStart: true,               // Start service with a delay after other auto-start services are started
		DisplayName:      ws.Config.DisplayName,
	}, ws.Config.Arguments...)

	if err != nil {
		return err
	}
	defer s.Close()
	return nil
}

func (ws *WindowsService) Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Config.Name)
	if err != nil {
		return fmt.Errorf("Could not access service %s: %v", ws.Config.Name, err)
	}
	defer s.Close()

	err = s.Delete()
	if err != nil {
		return err
	}
	return nil
}

func (ws *WindowsService) Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Config.Name)
	if err != nil {
		return fmt.Errorf("Could not access service %s: %v", ws.Config.Name, err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("Could not start service %s: %v", ws.Config.Name, err)
	}
	return nil
}

func (ws *WindowsService) Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Config.Name)
	if err != nil {
		return fmt.Errorf("Could not access service %s: %v", ws.Config.Name, err)
	}
	defer s.Close()

	err = ws.handleStop(s)
	if err != nil {
		return fmt.Errorf("Could not stop service %s: %v", ws.Config.Name, err)
	}
	return nil
}

func (ws *WindowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
loop:
	for {
		c := <-r
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop:
			// TODO: Determine if process clean up needs to take place here
			// Or whether we want to log any output
			break loop
		case svc.Shutdown:
			// TODO: Determine if process clean up needs to take place here
			break loop
		default:
			continue loop
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	return false, 0
}

func (ws *WindowsService) IsInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Config.Name)
	defer s.Close()
	return err == nil
}

func (ws *WindowsService) IsRunning() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.Config.Name)
	if err != nil {
		return false
	}
	defer s.Close()

	status, err := s.Query()
	return status.State == svc.Running
}

func (ws *WindowsService) handleStop(s *mgr.Service) error {
	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("Could not stop service %s: %v", ws.Config.Name, err)
	}

	timeout := time.Now().Add(ServiceStopTimeout)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return errors.New("Timeout waiting for service to go to 'Stop' state")
		}

		time.Sleep(ServiceStopStatusCheckInterval)

		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("Could not retrieve service status: %v", err)
		}
	}
	return nil
}
