package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	handlersettings "github.com/Azure/azure-extension-platform/pkg/settings"
	"github.com/Azure/azure-extension-platform/vmextension"
	"github.com/stretchr/testify/assert"
)

// implements IHostGaCommunicator
type NoopHostGaCommunicator struct {
	MyApp *hostgacommunicator.VMAppMetadata
}

func (communicator *NoopHostGaCommunicator) DownloadPackage(el *logging.ExtensionLogger, appName string, dst string) error {
	return nil
}
func (communicator *NoopHostGaCommunicator) DownloadConfig(el *logging.ExtensionLogger, appName string, dst string) error {
	return nil
}
func (communicator *NoopHostGaCommunicator) GetVMAppInfo(el *logging.ExtensionLogger, appName string) (*hostgacommunicator.VMAppMetadata, error) {
	return communicator.MyApp, nil
}

func (communicator *NoopHostGaCommunicator) SetupVMAppInfo(appName string, version string) {
	communicator.MyApp = &hostgacommunicator.VMAppMetadata{
		ApplicationName:    appName,
		DirectDownloadOnly: false,
		InstallCommand:     "",
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
	cleanTestDir()

	os.Exit(exitVal)
}

func TestSettingsFailToInit(t *testing.T) {
	extensionVersion = ""
	defer resetExtensionVersion()
	err := getExtensionAndRun()
	assert.Error(t, err)
}

func TestFailToCreateExtension(t *testing.T) {
	// This will fail automatically because Guest Agent hasn't set the required sequence numbers
	err := getExtensionAndRun()
	assert.Error(t, err)
}

func TestGetVMPackageDataNoSettings(t *testing.T) {
	ext := createTestVMExtension(t, nil)
	_, err := vmAppEnableCallback(ext)
	assert.Error(t, err)
}

func TestGetVMPackageDataCannotDeserialize(t *testing.T) {
	vmPackages := "yabasnarfle {}"

	ext := createTestVMExtension(t, vmPackages)
	_, err := vmAppEnableCallback(ext)
	assert.Error(t, err)
}

func TestGetVMPackageDataNoApplications(t *testing.T) {
	vmApplications := []VmAppSetting{}

	ext := createTestVMExtension(t, vmApplications)
	_, err := vmAppEnableCallback(ext)
	assert.NoError(t, err)
}

func TestEnableCallbackWithOneApplication(t *testing.T) {
	defer cleanTestDir()
	order := 1
	vmApplications := []VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1")
	statusString, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	assert.NoError(t, err, "enable callback should return no error")
	statusMessage := new(StatusMessageWithPackageOperationResults)
	err = json.Unmarshal([]byte(statusString), statusMessage)
	assert.NoError(t, err, "statusString should deserialize to an object of type StatusMessageWithPackageOperationResults")
	assert.Equal(t, 1, len(statusMessage.ActionsPerformed), "there should be one action performed")
	assert.Equal(t, actionplan.Success, statusMessage.ActionsPerformed[0].Result)
}

func TestTreatFailureAsDeploymentFailure(t *testing.T) {
	defer cleanTestDir()
	order := 1
	vmApplications := []VmAppSetting{
		{
			ApplicationName:                 "iggy",
			Order:                           &order,
			TreatFailureAsDeploymentFailure: true,
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1")
	hostGaCommunicator.MyApp.InstallCommand = "return 1"
	statusString, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	assert.EqualError(t, err, treatFailureAsDeploymentFailureError.Error(), "enable callback should return expected error")
	statusMessage := new(StatusMessageWithPackageOperationResults)
	err = json.Unmarshal([]byte(statusString), statusMessage)
	assert.NoError(t, err, "statusString should deserialize to an object of type StatusMessageWithPackageOperationResults")
	assert.Equal(t, 1, len(statusMessage.ActionsPerformed), "there should be one action performed")
	assert.Contains(t, statusMessage.ActionsPerformed[0].Result, hostGaCommunicator.MyApp.InstallCommand)
	assert.Equal(t, statusMessage.ActionsPerformed[0].Operation, packageregistry.Install.ToString())

	// test for update
	cleanTestDir()
	ext = createTestVMExtension(t, vmApplications)
	hostGaCommunicator.MyApp.InstallCommand = "return 0"
	// let install succeed, next enable should call update
	_, _ = doVmAppEnableCallback(ext, &hostGaCommunicator)
	hostGaCommunicator.MyApp.UpdateCommand = "return 1"
	hostGaCommunicator.MyApp.Version = "1.0.2"

	statusString, err = doVmAppEnableCallback(ext, &hostGaCommunicator)
	assert.EqualError(t, err, treatFailureAsDeploymentFailureError.Error(), "enable callback should return expected error")
	statusMessage = new(StatusMessageWithPackageOperationResults)
	err = json.Unmarshal([]byte(statusString), statusMessage)
	assert.NoError(t, err, "statusString should deserialize to an object of type StatusMessageWithPackageOperationResults")
	assert.Equal(t, 1, len(statusMessage.ActionsPerformed), "there should be one action performed")
	assert.Contains(t, statusMessage.ActionsPerformed[0].Result, hostGaCommunicator.MyApp.UpdateCommand)
	assert.Equal(t, statusMessage.ActionsPerformed[0].Operation, packageregistry.Update.ToString())

	// also test enable callback return no error when TreatFailureAsDeploymentFailure is false
	cleanTestDir()
	vmApplications[0].ApplicationName = "app2"
	vmApplications[0].TreatFailureAsDeploymentFailure = false
	hostGaCommunicator.MyApp.InstallCommand = "return 2"
	ext = createTestVMExtension(t, vmApplications)
	statusString, err = doVmAppEnableCallback(ext, &hostGaCommunicator)
	assert.NoError(t, err, "enableCallBack should not return error even on failure of install command when TreatFailureAsDeploymentFailure is false")
	statusMessage = new(StatusMessageWithPackageOperationResults)
	err = json.Unmarshal([]byte(statusString), statusMessage)
	assert.NoError(t, err, "statusString should deserialize to an object of type StatusMessageWithPackageOperationResults")
	assert.Equal(t, 1, len(statusMessage.ActionsPerformed), "there should be one action performed")
	assert.Contains(t, statusMessage.ActionsPerformed[0].Result, hostGaCommunicator.MyApp.InstallCommand)
}

func TestGetVMPackageDataNoVersion(t *testing.T) {
	order := 1
	vmApplications := []VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}

	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "")
	ext := createTestVMExtension(t, vmApplications)
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	assert.Error(t, err)
}

func TestGetVMPackageDataNoApplicationName(t *testing.T) {
	order := 1
	vmApplications := []VmAppSetting{
		{
			ApplicationName: "",
			Order:           &order,
		},
	}

	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1")
	ext := createTestVMExtension(t, vmApplications)
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	assert.Error(t, err)
}

func TestEnableCallbackNothingToProcess(t *testing.T) {
	vmApplications := []VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)

	hostGaCommunicator := NoopHostGaCommunicator{}
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	assert.NoError(t, err)
}

func cleanTestDir() {
	os.RemoveAll(maintestdir)
}

func resetExtensionVersion() {
	extensionVersion = "1.0.0"
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
	assert.NoError(t, err)

	el := logging.New(nil)
	he := &handlerenv.HandlerEnvironment{
		HeartbeatFile: path.Join(maintestdir, "heartbeat.txt"),
		StatusFolder:  path.Join(maintestdir, "status/"),
		ConfigFolder:  configFolder,
		LogFolder:     path.Join(maintestdir, "log/"),
		DataFolder:    path.Join(maintestdir, "data/"),
	}
	eem := extensionevents.New(el, he)

	return &vmextension.VMExtension{
		Name:                       extensionVersion,
		Version:                    extensionVersion,
		GetRequestedSequenceNumber: func() (uint, error) { return 2, nil },
		CurrentSequenceNumber:      &one,
		HandlerEnv:                 he,
		GetSettings: func() (*handlersettings.HandlerSettings, error) {
			return hs, nil
		},
		ExtensionLogger: el,
		ExtensionEvents: eem,
	}
}
