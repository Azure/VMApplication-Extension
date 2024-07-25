package service

import "os"

type LinuxServiceManager struct{}

type LinuxService struct {
	Config *ServiceConfig
}

func (LinuxServiceManager) String() string {
	return "linux-systemd"
}

func (LinuxServiceManager) DetectIsAvailable() bool {
	return isSystemdAvailable()
}

func (LinuxServiceManager) New(c *ServiceConfig) (Service, error) {
	service := &LinuxService{
		Config: c,
	}
	return service, nil
}

func (ls *LinuxService) Install() error {
	return nil
}

func (ls *LinuxService) Uninstall() error {
	return nil
}

func (ls *LinuxService) Start() error {
	return nil
}

func (ls *LinuxService) Stop() error {
	return nil
}

func (ls *LinuxService) Restart() error {
	return nil
}

func (ls *LinuxService) IsActive() bool {
	return false
}

func isSystemdAvailable() bool {
	// Check if /run/systemd/system exists, if so we have systemd
	info, err := os.Stat("/run/systemd/system")
	return err == nil && info.IsDir()
}
