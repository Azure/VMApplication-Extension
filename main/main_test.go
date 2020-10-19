package main

import (
	"encoding/json"
	"os"
	"testing"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper/handlerenv"
	handlersettings "github.com/Azure/VMApplication-Extension/VmExtensionHelper/settings"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper/vmextension"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

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
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(nil)
	_, err := vmAppEnableCallback(ctx, ext)
	require.Error(t, err)
}

func Test_getVMPackageData_cannotDeserialize(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := "yabasnarfle {}"

	ext := createTestVMExtension(vmPackages)
	_, err := vmAppEnableCallback(ctx, ext)
	require.Error(t, err)
}

func Test_getVMPackageData_noApplications(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := vmPackageData{
		Packages: []vmPackage{},
	}

	ext := createTestVMExtension(vmPackages)
	_, err := vmAppEnableCallback(ctx, ext)
	require.NoError(t, err)
}

func Test_getVMPackageData_valid(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := vmPackageData{
		Packages: []vmPackage{
			{
				Name:      "iggy",
				Operation: "install",
				Version:   "1.0.0",
			},
		},
	}

	ext := createTestVMExtension(vmPackages)
	_, err := vmAppEnableCallback(ctx, ext)
	require.NoError(t, err)
}

func Test_getVMPackageData_noVersion(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := vmPackageData{
		Packages: []vmPackage{
			{
				Name:      "iggy",
				Operation: "install",
				Version:   "",
			},
		},
	}

	ext := createTestVMExtension(vmPackages)
	_, err := vmAppEnableCallback(ctx, ext)
	require.Error(t, err)
}

func Test_getVMPackageData_noOperationName(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := vmPackageData{
		Packages: []vmPackage{
			{
				Name:      "iggy",
				Operation: "",
				Version:   "1.0.0",
			},
		},
	}

	ext := createTestVMExtension(vmPackages)
	_, err := vmAppEnableCallback(ctx, ext)
	require.Error(t, err)
}

func Test_getVMPackageData_noApplicationName(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := vmPackageData{
		Packages: []vmPackage{
			{
				Name:      "",
				Operation: "install",
				Version:   "1.0.0",
			},
		},
	}

	ext := createTestVMExtension(vmPackages)
	_, err := vmAppEnableCallback(ctx, ext)
	require.Error(t, err)
}

func Test_main_getPackageStatePlanFails(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := createVmPackageData()

	ext := createTestVMExtension(vmPackages)
	osDependency = NewMockDependencies()
	defer resetOSDependency()
	_, err := vmAppEnableCallback(ctx, ext)
	require.Error(t, err)
}

func Test_main_nothingToProcess(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := vmPackageData{
		Packages: []vmPackage{},
	}
	ext := createTestVMExtension(vmPackages)

	result, err := vmAppEnableCallback(ctx, ext)
	require.NoError(t, err)
	require.Equal(t, "Nothing to process", result)
}

func Test_main_processPackagesNormal(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := createMultipleVmPackageData()
	ext := createTestVMExtension(vmPackages)

	result, err := vmAppEnableCallback(ctx, ext)
	require.NoError(t, err)
	require.Equal(t, "Complete", result)
}

func Test_main_processPackagesFailToMark(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	vmPackages := createMultipleVmPackageData()
	ext := createTestVMExtension(vmPackages)

	mockDependency := NewBareMockDependencies()
	osDependency = mockDependency
	mockDependency.UseMockRemoveFile = true
	defer resetOSDependency()
	_, err := vmAppEnableCallback(ctx, ext)
	require.Error(t, err)
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
	publicmap := make(map[string]interface{}, 1)
	privatemap := make(map[string]interface{}, 2)

	if settings != nil {
		b, _ := json.Marshal(settings)
		privatemap[vmPackagesSetting] = string(b)
	}

	return &handlersettings.HandlerSettings{
		PublicSettings:    publicmap,
		ProtectedSettings: privatemap,
	}
}

func createTestVMExtension(settings interface{}) *vmextension.VMExtension {
	hs := createSettings(settings)

	return &vmextension.VMExtension{
		Name:                    extensionVersion,
		Version:                 extensionVersion,
		RequestedSequenceNumber: 2,
		CurrentSequenceNumber:   1,
		HandlerEnv: &handlerenv.HandlerEnvironment{
			HeartbeatFile: "./heartbeat.txt",
			StatusFolder:  "./status/",
			ConfigFolder:  "./config/",
			LogFolder:     "./log/",
			DataFolder:    "./data/",
		},
		Settings: hs,
	}
}
