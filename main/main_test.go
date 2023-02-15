package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	handlersettings "github.com/Azure/azure-extension-platform/pkg/settings"
	"github.com/Azure/azure-extension-platform/pkg/status"
	"github.com/Azure/azure-extension-platform/vmextension"
	"github.com/stretchr/testify/assert"
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
		RemoveCommand:      "",
		UpdateCommand:      "",
		Version:            version,
	}
}

var noopHostGaCommunicator = new(NoopHostGaCommunicator)

var sequenceNumberSetByTheExtension uint

func nopLog() *logging.ExtensionLogger {
	return logging.New(nil)
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

	sequenceNumberSetByTheExtension = 0
	setSequenceNumberFunc = func(extName, extVersion string, seqNo uint) error {
		sequenceNumberSetByTheExtension = seqNo
		return nil
	}

	maintestdir = testdir
	exitVal := m.Run()
	os.RemoveAll(maintestdir)

	os.Exit(exitVal)
}

func Test_settingsFailToInit(t *testing.T) {
	ExtensionVersion = ""
	defer resetExtensionVersion()
	err := getExtensionAndRun([]string{"vm-application-manager", "enable"})
	require.Error(t, err)
}

func Test_failToCreateExtension(t *testing.T) {
	// This will fail automatically because Guest Agent hasn't set the required sequence numbers
	err := getExtensionAndRun([]string{"vm-application-manager", "enable"})
	require.Error(t, err)
}

func Test_getVMPackageData_noSettings(t *testing.T) {
	ext := createTestVMExtension(t, nil)
	err := customEnable(ext, noopHostGaCommunicator, 0)
	require.Error(t, err)
}

func Test_getVMPackageData_cannotDeserialize(t *testing.T) {
	vmPackages := "yabasnarfle {}"

	ext := createTestVMExtension(t, vmPackages)
	err := customEnable(ext, noopHostGaCommunicator, 0)
	require.Error(t, err)
}

func Test_getVMPackageData_noApplications(t *testing.T) {
	vmApplications := []extdeserialization.VmAppSetting{}

	ext := createTestVMExtension(t, vmApplications)
	err := customEnable(ext, noopHostGaCommunicator, 0)
	require.NoError(t, err)
}

func Test_getVMPackageData_valid(t *testing.T) {
	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	err := customEnable(ext, &hostGaCommunicator, 0)
	require.NoError(t, err)
}

func Test_getVMAppProtectedSettings_valid(t *testing.T) {
	order := 1
	actions := extdeserialization.ActionSetting{
		ActionName:   "logging",
		ActionScript: "echo %CustomAction_blobURL%",
		Timestamp:    "20210604T155300Z",
		Parameters: []extdeserialization.ActionParameter{
			{
				ParameterName:  "blobURL",
				ParameterValue: "myaccount.blob.core.windows.net",
			},
		},
		TickCount: 10193113,
	}
	appSettings := extdeserialization.VmAppSetting{
		ApplicationName: "iggy",
		Order:           &order,
		Actions:         []*extdeserialization.ActionSetting{&actions},
	}
	vmAppProtectedSettings := extdeserialization.VmAppProtectedSettings{&appSettings}
	testSettings := handlersettings.HandlerSettings{
		PublicSettings:    "{}",
		ProtectedSettings: "[{\"applicationName\": \"iggy\", \"order\": 1, \"actions\": [{\"name\": \"logging\",\"script\": \"echo %CustomAction_blobURL%\",\"timestamp\": \"20210604T155300Z\",\"parameters\": [{\"name\": \"blobURL\",\"value\": \"myaccount.blob.core.windows.net\"}],\"tickCount\": 10193113}]}]",
	}

	out, err := extdeserialization.GetVMAppProtectedSettings(&testSettings)
	require.NoError(t, err)

	require.EqualValues(t, vmAppProtectedSettings[0].ApplicationName, out[0].ApplicationName)
	require.EqualValues(t, *vmAppProtectedSettings[0].Order, *out[0].Order)
	require.EqualValues(t, *vmAppProtectedSettings[0].Actions[0], *out[0].Actions[0])
}

func Test_getVMAppProtectedSettings_valid_no_custom_actions(t *testing.T) {
	order := 1

	appSettings := extdeserialization.VmAppSetting{
		ApplicationName: "iggy",
		Order:           &order,
	}
	vmAppProtectedSettings := extdeserialization.VmAppProtectedSettings{&appSettings}
	testSettings := handlersettings.HandlerSettings{
		PublicSettings:    "{}",
		ProtectedSettings: "[{\"applicationName\": \"iggy\", \"order\": 1, \"tickCount\": 10193113}]",
	}

	out, err := extdeserialization.GetVMAppProtectedSettings(&testSettings)
	require.NoError(t, err)

	require.EqualValues(t, vmAppProtectedSettings[0].ApplicationName, out[0].ApplicationName)
	require.EqualValues(t, *vmAppProtectedSettings[0].Order, *out[0].Order)
}

func Test_getVMPackageData_noVersion(t *testing.T) {
	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}

	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "", "install")
	ext := createTestVMExtension(t, vmApplications)
	err := customEnable(ext, &hostGaCommunicator, 0)
	require.Error(t, err)
}

func Test_getVMPackageDataCustomAction_valid(t *testing.T) {
	order := 1
	actions := extdeserialization.ActionSetting{
		ActionName:   "Action1",
		ActionScript: "echo hello",
		Timestamp:    "20210604T155300Z",
		Parameters:   []extdeserialization.ActionParameter{},
		TickCount:    12346578,
	}
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
			Actions:         []*extdeserialization.ActionSetting{&actions},
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	err := customEnable(ext, &hostGaCommunicator, 0)
	require.NoError(t, err)
	// test that registry file is written
	pkr, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, 1*time.Second)
	require.NoError(t, err)
	currentpackages, err := pkr.GetExistingPackages()
	require.NoError(t, err)
	require.Len(t, currentpackages, 1)
	require.Equal(t, currentpackages[vmApplications[0].ApplicationName].OngoingOperation, packageregistry.NoAction)
	require.Contains(t, currentpackages[vmApplications[0].ApplicationName].Result, actionplan.Success)
}

func Test_getVMPackageDataCustomAction_CriticalError(t *testing.T) {
	order := 1
	actions := extdeserialization.ActionSetting{
		ActionName:   "Action1",
		ActionScript: "echo hello",
		Timestamp:    "20210604T155300Z",
		Parameters:   []extdeserialization.ActionParameter{},
		TickCount:    12346578,
	}
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "",
			Order:           &order,
			Actions:         []*extdeserialization.ActionSetting{&actions},
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	err := customEnable(ext, &hostGaCommunicator, 0)
	require.Error(t, err)
}

func Test_getVMPackageData_noApplicationName(t *testing.T) {
	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "",
			Order:           &order,
		},
	}

	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	ext := createTestVMExtension(t, vmApplications)
	err := customEnable(ext, &hostGaCommunicator, 0)
	require.Error(t, err)
}

func Test_main_statusIsWrittenForCriticalErrors(t *testing.T) {
	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "",
			Order:           &order,
		},
	}

	requestedSequenceNumber := uint(5)
	oldGetVMExtFunc := getVMExtensionFunc
	var ext *vmextension.VMExtension
	getVMExtensionFunc = func() (*vmextension.VMExtension, error) {
		ext = createTestVMExtension(t, vmApplications)
		ext.GetRequestedSequenceNumber = func() (uint, error) { return requestedSequenceNumber, nil }
		return ext, nil
	}
	defer func() {
		getVMExtensionFunc = oldGetVMExtFunc
	}()

	err := getExtensionAndRun([]string{"vm-application-manager", vmextension.EnableOperation.ToString()})
	require.NoError(t, err)
	statusFilePath := filepath.Join(ext.HandlerEnv.StatusFolder, fmt.Sprintf("%d.status", requestedSequenceNumber))
	fileBytes, err := ioutil.ReadFile(statusFilePath)
	require.NoError(t, err)
	fileString := string(fileBytes)
	require.Contains(t, fileString, vmextension.EnableOperation.ToStatusName())
	require.Contains(t, fileString, status.StatusError)
	// test that the sequence number isn't updated
	// extension will retry the sequence number is the action plan could not be executed
	require.NotEqual(t, requestedSequenceNumber, sequenceNumberSetByTheExtension)

}

func Test_main_nothingToProcess_withoutStatus(t *testing.T) {
	vmApplications := []extdeserialization.VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)

	hostGaCommunicator := NoopHostGaCommunicator{}
	requestedSequenceNumber := uint(0)
	err := customEnable(ext, &hostGaCommunicator, requestedSequenceNumber)
	require.NoError(t, err)
	// ensure stautus file is not written
	statusFilePath := filepath.Join(ext.HandlerEnv.StatusFolder, fmt.Sprintf("%d.status", requestedSequenceNumber))
	require.False(t, fileExists(statusFilePath))
	require.Equal(t, requestedSequenceNumber, sequenceNumberSetByTheExtension)
}

func Test_main_nothingToProcess_withStatus(t *testing.T) {
	vmApplications := []extdeserialization.VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	requestedSequenceNumber := *ext.CurrentSequenceNumber + 1
	err := customEnable(ext, &hostGaCommunicator, requestedSequenceNumber)
	require.NoError(t, err)
	statusFilePath := filepath.Join(ext.HandlerEnv.StatusFolder, fmt.Sprintf("%d.status", requestedSequenceNumber))
	fileBytes, err := ioutil.ReadFile(statusFilePath)
	require.NoError(t, err)
	fileString := string(fileBytes)
	require.Contains(t, fileString, vmextension.EnableOperation.ToStatusName())
	require.Contains(t, fileString, status.StatusSuccess)
	require.Equal(t, requestedSequenceNumber, sequenceNumberSetByTheExtension)
}

func Test_uninstall_cannotCreatePackageRegistry(t *testing.T) {
	vmApplications := []extdeserialization.VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}

	// Set the config folder to an invalid path so we can't create a package registry
	ext.HandlerEnv.ConfigFolder = "/yabaflarg/flarpaglarp"

	err := doVmAppUninstallCallback(ext, &hostGaCommunicator)
	require.Error(t, err)
	require.EqualError(t, err, cannotCreatePackageRegistryError)
}

func Test_uninstall_cannotReadPackageRegistry(t *testing.T) {
	vmApplications := []extdeserialization.VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}

	// Write an invalid registry so we can't create a package registry
	appRegistryFilePath := path.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	ioutil.WriteFile(appRegistryFilePath, []byte("}"), 0644)
	defer os.Remove(appRegistryFilePath)

	err := doVmAppUninstallCallback(ext, &hostGaCommunicator)
	require.Error(t, err)
	require.EqualError(t, err, "could not read current package registry: invalid character '}' looking for beginning of value")
}

func Test_uninstall_noAppsToUninstall(t *testing.T) {
	vmApplications := []extdeserialization.VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}

	package1 := path.Join(ext.HandlerEnv.ConfigFolder, "package1")
	package2 := path.Join(ext.HandlerEnv.ConfigFolder, "package2")
	package1Quotes := fmt.Sprintf("\"%v\"", package1)
	package2Quotes := fmt.Sprintf("\"%v\"", package2)

	// Create a package registry where the remove commands will write their respective files
	reg := packageregistry.CurrentPackageRegistry{"package1": &packageregistry.VMAppPackageCurrent{
		ApplicationName:    "package1",
		DirectDownloadOnly: false,
		InstallCommand:     "dontcare",
		RemoveCommand:      "echo moein > " + package1Quotes,
		UpdateCommand:      "dontcare",
		Version:            "1.2.3.1",
	}, "package2": &packageregistry.VMAppPackageCurrent{
		ApplicationName:    "package2",
		DirectDownloadOnly: true,
		InstallCommand:     "dontcare",
		RemoveCommand:      "echo moein > " + package2Quotes,
		UpdateCommand:      "dontcare",
		Version:            "1.2.3.2",
	}}

	pkgHndlr, err := packageregistry.New(nopLog(), ext.HandlerEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(reg)
	assert.NoError(t, err, "Should be able to write package registry to disk")
	pkgHndlr.Close()

	err = doVmAppUninstallCallback(ext, &hostGaCommunicator)
	require.NoError(t, err)

	// Verify we removed both apps, which deleted the files
	require.True(t, fileExists(package1), "First application was not removed")
	require.True(t, fileExists(package2), "Second application was not removed")
}

func Test_uninstall_uninstallApps(t *testing.T) {
	vmApplications := []extdeserialization.VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}

	err := doVmAppUninstallCallback(ext, &hostGaCommunicator)
	require.NoError(t, err)
}

func fileExists(filePath string) bool {
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

func resetExtensionVersion() {
	ExtensionVersion = "1.0.0"
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

	configFolder := path.Join(maintestdir, "config")
	err := os.MkdirAll(configFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	require.NoError(t, err)

	el := logging.New(nil)
	he := &handlerenv.HandlerEnvironment{
		HeartbeatFile: path.Join(maintestdir, "heartbeat.txt"),
		StatusFolder:  path.Join(maintestdir, "status"),
		ConfigFolder:  configFolder,
		LogFolder:     path.Join(maintestdir, "log"),
		DataFolder:    path.Join(maintestdir, "data"),
	}
	err = os.MkdirAll(he.StatusFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	require.NoError(t, err)

	eem := extensionevents.New(el, he)

	return &vmextension.VMExtension{
		Name:                       ExtensionVersion,
		Version:                    ExtensionVersion,
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
