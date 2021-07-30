package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/Azure/VMApplication-Extension/internal/customactionplan"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
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
	vmApplications := []customactionplan.VmAppSetting{}

	ext := createTestVMExtension(t, vmApplications)
	_, err := vmAppEnableCallback(ext)
	require.NoError(t, err)
}

func Test_getVMPackageData_valid(t *testing.T) {
	order := 1
	vmApplications := []customactionplan.VmAppSetting{
		customactionplan.VmAppSetting{
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

func Test_getVMAppProtectedSettings_valid(t *testing.T) {
	order := 1
	actions := customactionplan.ActionSetting {
		ActionName: "logging",
		ActionScript: "echo %CustomAction_blobURL%",
		Timestamp: "20210604T155300Z",
		Parameters: []customactionplan.ActionParameter{
			{
				ParameterName: "blobURL",
				ParameterValue: "myaccount.blob.core.windows.net",
			},
		},
		TickCount: 10193113,
	}
	appSettings := customactionplan.VmAppSetting{
		ApplicationName: "iggy",
		Order: &order,
		Actions: []*customactionplan.ActionSetting {&actions},

	}
	vmAppProtectedSettings := VmAppProtectedSettings{&appSettings}
	testSettings := handlersettings.HandlerSettings {
		PublicSettings: "{}",
		ProtectedSettings: "[{\"name\": \"iggy\", \"order\": 1, \"actions\": [{\"name\": \"logging\",\"script\": \"echo %CustomAction_blobURL%\",\"timestamp\": \"20210604T155300Z\",\"parameters\": [{\"name\": \"blobURL\",\"value\": \"myaccount.blob.core.windows.net\"}],\"tickCount\": 10193113}]}]",
	}

	out, err := getVMAppProtectedSettings(&testSettings)
	require.NoError(t, err)

	require.EqualValues(t, vmAppProtectedSettings[0].ApplicationName, out[0].ApplicationName)
	require.EqualValues(t, *vmAppProtectedSettings[0].Order, *out[0].Order)
	require.EqualValues(t, *vmAppProtectedSettings[0].Actions[0], *out[0].Actions[0])

}

func Test_getVMPackageData_noVersion(t *testing.T) {
	order := 1
	vmApplications := []customactionplan.VmAppSetting{
		customactionplan.VmAppSetting{
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

func Test_getVMPackageDataCustomAction_valid(t *testing.T) {
	order := 1
	actions := customactionplan.ActionSetting {
				ActionName: "Action1",
				ActionScript: "echo hello",
				Timestamp: "20210604T155300Z",
				Parameters: []customactionplan.ActionParameter{},
				TickCount: 12346578,
		}
	vmApplications := []customactionplan.VmAppSetting{
		customactionplan.VmAppSetting{
			ApplicationName: "iggy",
			Order:           &order,
			Actions:		[]*customactionplan.ActionSetting{&actions},
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.NoError(t, err)
}

func Test_getVMPackageDataCustomAction_CriticalError(t *testing.T) {
	order := 1
	actions := customactionplan.ActionSetting {
		ActionName: "Action1",
		ActionScript: "echo hello",
		Timestamp: "20210604T155300Z",
		Parameters: []customactionplan.ActionParameter{},
		TickCount: 12346578,
	}
	vmApplications := []customactionplan.VmAppSetting{
		customactionplan.VmAppSetting{
			ApplicationName: "",
			Order: &order,
			Actions:		[]*customactionplan.ActionSetting{&actions},
		},
	}

	ext := createTestVMExtension(t, vmApplications)
	hostGaCommunicator := NoopHostGaCommunicator{}
	hostGaCommunicator.SetupVMAppInfo("iggy", "1.0.1", "install")
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.Error(t, err)
}

func Test_getVMPackageData_noApplicationName(t *testing.T) {
	order := 1
	vmApplications := []customactionplan.VmAppSetting{
		customactionplan.VmAppSetting{
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
	vmApplications := []customactionplan.VmAppSetting{}
	ext := createTestVMExtension(t, vmApplications)

	hostGaCommunicator := NoopHostGaCommunicator{}
	_, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	require.NoError(t, err)
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
	require.NoError(t, err)

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
