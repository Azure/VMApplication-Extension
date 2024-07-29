package service

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
)

const (
	LinuxServiceName                     = "linux-systemd"
	PreferredUnitConfigurationBasePath   = "/etc/systemd/system"           // Units installed by the system administrator
	AlternativeUnitConfigurationBasePath = "/usr/local/lib/systemd/system" // Units provided by installed packages
	SystemCtl                            = "systemctl"
	SystemCtlDaemonReload                = "daemon-reload"
	SystemCtlEnable                      = "enable"
	SystemCtlDisable                     = "disable"
	SystemCtlIsActive                    = "is-active"
	SystemCtlIsEnabled                   = "is-enabled"
	SystemCtlStart                       = "start"
	SystemCtlStop                        = "stop"
	SystemCtlStatus                      = "status"
	UnitConfigurationFileExtension       = ".service"
	UnitConfigurationFilePermission      = 0644
)

type LinuxServiceManager struct{}

type LinuxService struct {
	Config          *ServiceConfig
	ExtensionEvents *extensionevents.ExtensionEventManager // Allows extension to raise events
	ExtensionLogger *logging.ExtensionLogger               // Automatically logs to the log directory
}

func (LinuxServiceManager) String() string {
	return LinuxServiceName
}

func (LinuxServiceManager) DetectIsAvailable() bool {
	return isSystemdAvailable()
}

func (LinuxServiceManager) New(c *ServiceConfig, eem *extensionevents.ExtensionEventManager, el *logging.ExtensionLogger) (Service, error) {
	if len(c.Name) == 0 {
		return nil, errors.New("Name field within ServiceConfig is required")
	} else if len(c.UnitContent) == 0 {
		return nil, errors.New("UnitContent field within ServiceConfig is required")
	}

	service := &LinuxService{
		Config:          c,
		ExtensionEvents: eem,
		ExtensionLogger: el,
	}
	return service, nil
}

func (ls *LinuxService) Install() error {
	unitName := ls.unitName()

	err := removeUnitConfigurationFile(unitName, ls.ExtensionLogger)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Error while removing old unit configuration file: %v", err)
	}

	err := createUnitConfigurationFile(unitName, ls.Config.UnitContent, ls.ExtensionLogger)
	if err != nil {
		return err
	}

	// Reload systemd manager configuration and unit file
	err := ls.runSystemCtlAction(SystemCtlDaemonReload)
	if err != nil {
		return err
	}

	return ls.runSystemCtlAction(SystemCtlEnable)
}

func (ls *LinuxService) Uninstall() error {
	err := ls.runSystemCtlAction(SystemCtlDisable)
	if err != nil {
		return fmt.Errorf("Could not disable service %s: %v", ls.Config.Name, err)
	}

	unitName := ls.unitName()
	err := removeUnitConfigurationFile(unitName, ls.ExtensionLogger)
	if err != nil {
		return fmt.Errorf("Could not remove unit file %s: %v", unitName, err)
	}

	return nil
}

func (ls *LinuxService) Start() error {
	err := ls.runSystemCtlAction(SystemCtlStart)
	if err != nil {
		return fmt.Errorf("Could not start service %s: %v", ls.Config.Name, err)
	}

	return nil
}

func (ls *LinuxService) Stop() error {
	err := ls.runSystemCtlAction(SystemCtlStop)
	if err != nil {
		return fmt.Errorf("Could not stop service %s: %v", ls.Config.Name, err)
	}

	return nil
}

func (ls *LinuxService) IsInstalled() (bool, error) {
	unitName := ls.unitName()
	unitConfigPath, err := getUnitConfigurationFilePath(unitName, ls.ExtensionLogger)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(unitConfigPath)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "Error occurred while checking existence for file %s: %v", unitName, err)
	}

	return true, nil
}

func (ls *LinuxService) IsRunning() bool {
	err := ls.runSystemCtlAction(SystemCtlIsActive)
	if err != nil {
		return false, err
	}

	return true, nil
}

func isSystemdAvailable() bool {
	// Check if /run/systemd/system exists, if so we have systemd
	info, err := os.Stat("/run/systemd/system")
	return err == nil && info.IsDir()
}

func getUnitConfigurationFilePath(unitName string, logger *logging.ExtensionLogger) (string, error) {
	systemDPath, err := getSystemDConfigurationBasePath(logger)
	if err != nil {
		return nil, err
	}

	return path.Join(systemDPath, unitName), nil
}

func getSystemDConfigurationBasePath(logger *logging.ExtensionLogger) (string, error) {
	logger.Info("Getting systemd configuration path available in the system")
	info, err := os.Stat(PreferredUnitConfigurationBasePath)
	if err != nil || info == nil || !info.IsDir() {
		logger.Info("%v path was not found on the system. Attempt finding alternate path %v",
			PreferredUnitConfigurationBasePath, AlternativeUnitConfigurationBasePath)

		info, err = os.Stat(AlternativeUnitConfigurationBasePath)
		if err != nil || info == nil || !info.IsDir() {
			return nil, errors.New(fmt.Sprintf("Neither %s nor %s were found as directories on the system", PreferredUnitConfigurationBasePath, AlternativeUnitConfigurationBasePath))
		}

		logger.Info("Alternative path '%v' was found on the system", AlternativeUnitConfigurationBasePath)
		return AlternativeUnitConfigurationBasePath, nil
	}

	logger.Info("Preferred path '%v' was found on the system", PreferredUnitConfigurationBasePath)
	return PreferredUnitConfigurationBasePath, nil
}

func runAction(action string, args ...string) error {
	return exec.Command(SystemCtl, append([]string{action}, args...)...).Run()
}

func (ls *LinuxService) runSystemCtlAction(action string) error {
	return runAction(action, ls.Config.Name)
}

func (ls *LinuxService) unitName() string {
	return ls.Config.Name + UnitConfigurationFileExtension
}

func createUnitConfigurationFile(unitName string, content string, logger *logging.ExtensionLogger) error {
	unitConfigPath, err := getUnitConfigurationFilePath(unitName, logger)
	if err != nil {
		return nil, err
	}

	return os.WriteFile(unitConfigPath, content, UnitConfigurationFilePermission)
}

func removeUnitConfigurationFile(unitName string, logger *logging.ExtensionLogger) error {
	unitConfigPath, err := getUnitConfigurationFilePath(unitName, logger)
	if err != nil {
		return err
	}

	return os.Remove(unitConfigPath)
}
