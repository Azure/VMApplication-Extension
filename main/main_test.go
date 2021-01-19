package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	handlersettings "github.com/Azure/azure-extension-platform/pkg/settings"
	"github.com/Azure/azure-extension-platform/vmextension"
	"github.com/stretchr/testify/require"
)

// implements IHostGaCommunicator
type NoopHostGaCommunicator struct {
	myApp *hostgacommunicator.VMAppMetadata
}

func (communicator *NoopHostGaCommunicator) DownloadPackage(el *logging.ExtensionLogger, appName string, dst string) error {
	return nil
}
func (communicator *NoopHostGaCommunicator) DownloadConfig(el *logging.ExtensionLogger, appName string, dst string) error {
	return nil
}
func (communicator *NoopHostGaCommunicator) GetVMAppInfo(el *logging.ExtensionLogger, appName string) (*hostgacommunicator.VMAppMetadata, error) {
	return communicator.myApp, nil
}

func (communicator *NoopHostGaCommunicator) SetupVMAppInfo(appName string, version string, operation string) {
	communicator.myApp = &hostgacommunicator.VMAppMetadata{
		ApplicationName:    appName,
		DirectDownloadOnly: false,
		InstallCommand:     "",
		Operation:          operation,
		RemoveCommand:      "",
		UpdateCommand:      "",
		Version:            version,
	}
}

var maintestdir string

func TestMain(m *testing.M) {
	testdir, err := ioutil.TempDir("", "maintest")
	if err != nil {
		return
	}

	err = os.MkdirAll(testdir, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		return
	}

	maintestdir = testdir
	exitVal := m.Run()
	os.RemoveAll(maintestdir)

	os.Exit(exitVal)
}

func Test_settingsFailToInit(t *testing.T) {
	extensionVersion = ""
	defer resetExtensionVersion()
	err := getExtensionAndRun()
	require.Error(t, err)
}

func Test_failToCreateExtension(t *testing.T) {
	// This will fail automatically because Guest Agent hasn't set the required sequence numbers
	err := getExtensionAndRun()
	require.Error(t, err)
}

func Test_getVMPackageData_noSettings(t *testing.T) {
	ext := createTestVMExtension(t, nil)
	_, err := vmAppEnableCallback(ext)
	require.Error(t, err)
}

func Test_getVMPackageData_cannotDeserialize(t *testing.T) {
	vmPackages := "yabasnarfle {}"

	ext := createTestVMExtension(t, vmPackages)
	_, err := vmAppEnableCallback(ext)
	require.Error(t, err)
}

func Test_getVMPackageData_noApplications(t *testing.T) {
	vmApplications := []VmAppSetting{}

	ext := createTestVMExtension(t, vmApplications)
	_, err := vmAppEnableCallback(ext)
	require.NoError(t, err)
}

func Test_getVMPackageData_valid(t *testing.T) {
	order := 1
	vmApplications := []VmAppSetting{
		VmAppSetting{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.NoError(t, err)
}

func Test_getVMPackageData_noVersion(t *testing.T) {
	order := 1
	vmApplications := []VmAppSetting{
		VmAppSetting{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}

	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "", "install")
	ext := createTestVMExtension(t, vmApplications)
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.Error(t, err)
}

func Test_getVMPackageData_noOperationName(t *testing.T) {
	order := 1
	vmApplications := []VmAppSetting{
		VmAppSetting{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}

	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "")
	ext := createTestVMExtension(t, vmApplications)
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.Error(t, err)
}

func Test_getVMPackageData_noApplicationName(t *testing.T) {
	order := 1
	vmApplications := []VmAppSetting{
		VmAppSetting{
			ApplicationName: "",
			Order:           &order,
		},
	}

	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	ext := createTestVMExtension(t, vmApplications)
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.Error(t, err)
}

func Test_main_nothingToProcess(t *testing.T) {
	vmApplications := []VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)

	hostGaCommunicator := NoopHostGaCommunicator{}
	result, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.NoError(t, err)
	require.Equal(t, "Operation completed", result)
}

func resetExtensionVersion() {
	extensionVersion = "1.0.0"
}

func createVmPackageData() vmPackageData {
	vmPackages := vmPackageData{
		Packages: []vmPackage{
			{
				Name:      "yaba",
				Operation: "install",
				Version:   "1.0.0",
			},
		},
	}

	return vmPackages
}

func createMultipleVmPackageData() vmPackageData {
	vmPackages := vmPackageData{
		Packages: []vmPackage{
			{
				Name:      "yaba",
				Operation: "install",
				Version:   "1.0.0",
			},
			{
				Name:      "flipmonster",
				Operation: "enable",
				Version:   "1.0.0",
			},
		},
	}

	return vmPackages
}

func createSettings(settings interface{}) *handlersettings.HandlerSettings {
	if settings == nil {
		return &handlersettings.HandlerSettings{
			PublicSettings:    "",
			ProtectedSettings: "",
		}
	} else {
		b, _ := json.Marshal(settings)

		return &handlersettings.HandlerSettings{
			PublicSettings:    "",
			ProtectedSettings: string(b),
		}
	}
}

var one uint = 1

func createTestVMExtension(t *testing.T, settings interface{}) *vmextension.VMExtension {
	hs := createSettings(settings)

	configFolder := path.Join(maintestdir, "config/")
	err := os.MkdirAll(configFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	require.NoError(t, err)

	return &vmextension.VMExtension{
		Name:                    extensionVersion,
		Version:                 extensionVersion,
		RequestedSequenceNumber: 2,
		CurrentSequenceNumber:   &one,
		HandlerEnv: &handlerenv.HandlerEnvironment{
			HeartbeatFile: path.Join(maintestdir, "heartbeat.txt"),
			StatusFolder:  path.Join(maintestdir, "status/"),
			ConfigFolder:  configFolder,
			LogFolder:     path.Join(maintestdir, "log/"),
			DataFolder:    path.Join(maintestdir, "data/"),
		},
		Settings: hs,
	}
}
