package service

import (
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

func (ws *WindowsService) Restart() error {
	return nil
}

func (ws *WindowsService) IsActive() bool {
	return false
}

func (ws *WindowsService) handleStop(s *mgr.Service) error {
	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("Could not stop service %s: %v", ws.Config.Name, err)
	}

	timeout := time.Now().Add(ServiceStopTimeout)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}

		time.Sleep(ServiceStopStatusCheckInterval)

		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}
