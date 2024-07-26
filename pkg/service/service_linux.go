package service

import (
	"fmt"
	"os"
	"path"
)

const (
	LinuxServiceName                     = "linux-systemd"
	PreferredUnitConfigurationBasePath   = "/etc/systemd/system"           // Units installed by the system administrator
	AlternativeUnitConfigurationBasePath = "/usr/local/lib/systemd/system" // Units provided by installed packages
	SystemCtl                            = "systemctl"
	UnitConfigurationFileExtension       = ".service"
	UnitConfigurationFilePermission      = 0644
)

type LinuxServiceManager struct{}

type LinuxService struct {
	Config *ServiceConfig
}

func (LinuxServiceManager) String() string {
	return LinuxServiceName
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

func (ls *LinuxService) IsIntalled() bool {
	return false
}

func (ls *LinuxService) IsRunning() bool {
	return false
}

func isSystemdAvailable() bool {
	// Check if /run/systemd/system exists, if so we have systemd
	info, err := os.Stat("/run/systemd/system")
	return err == nil && info.IsDir()
}

func getUnitConfigurationFilePath(unitName string) (string, error) {
	systemDPath, err := getSystemDConfigurationBasePath()
	if err != nil {
		return nil, err
	}

	return path.Join(systemDPath, unitName), nil
}

func getSystemDConfigurationBasePath() (string, error) {
	// ctx.Log("message", "Getting systemd configuration path available in the system")
	info, err := os.Stat(PreferredUnitConfigurationBasePath)
	if err != nil || info == nil || !info.IsDir() {
		// ctx.Log("message", fmt.Sprintf("INFO: %s path was not found on the system", unitConfigurationBasePath_preferred))

		info, err = os.Stat(AlternativeUnitConfigurationBasePath)
		if err != nil || info == nil || !info.IsDir() {
			return nil, fmt.Errorf("Neither %s nor %s were found as directories on the system", PreferredUnitConfigurationBasePath, AlternativeUnitConfigurationBasePath)
		}

		// ctx.Log("message", fmt.Sprintf("Alternative path was found on the system: %s", unitConfigurationBasePath_alternative))
		return AlternativeUnitConfigurationBasePath, nil
	}

	// ctx.Log("message", fmt.Sprintf("Preferred path was found on the system: %s", unitConfigurationBasePath_preferred))
	return PreferredUnitConfigurationBasePath, nil
}

func (ls *LinuxService) runAction(action string, args ...string) error {
	return run(SystemCtl, append([]string{action}, args...)...)
}

func (ls *LinuxService) runActionForUnit(action string) error {
	return ls.runSystemCtlAction(action, ls.Config.Name)
}

func (ls *LinuxService) unitName() string {
	return ls.Config.Name + UnitConfigurationFileExtension
}

func createUnitConfigurationFile(unitName string, content string) error {
	// Create/override unit configuration file
	unitConfigPath, err := GetUnitConfigurationFilePath(unitName)
	if err != nil {
		return nil, err
	}

	return os.WriteFile(unitConfigPath, content, UnitConfigurationFilePermission)
}
