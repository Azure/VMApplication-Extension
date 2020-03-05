package main

import (
	"encoding/json"
	"os"
	"testing"

	vmextensionhelper "github.com/Azure/VMApplication-Extension/VmExtensionHelper"
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

func resetExtensionVersion() {
	extensionVersion = "1.0.0"
}

func createSettings(settings interface{}) *vmextensionhelper.HandlerSettings {
	publicmap := make(map[string]interface{}, 1)
	privatemap := make(map[string]interface{}, 2)

	if settings != nil {
		b, _ := json.Marshal(settings)
		privatemap[vmPackagesSetting] = string(b)
	}

	return &vmextensionhelper.HandlerSettings{
		PublicSettings:    publicmap,
		ProtectedSettings: privatemap,
	}
}

func createTestVMExtension(settings interface{}) *vmextensionhelper.VMExtension {
	hs := createSettings(settings)

	return &vmextensionhelper.VMExtension{
		Name:                    extensionVersion,
		Version:                 extensionVersion,
		RequestedSequenceNumber: 2,
		CurrentSequenceNumber:   1,
		HandlerEnv: &vmextensionhelper.HandlerEnvironment{
			HeartbeatFile: "./heartbeat.txt",
			StatusFolder:  "./status/",
			ConfigFolder:  "./config/",
			LogFolder:     "./log/",
			DataFolder:    "./data/",
		},
		Settings: hs,
	}
}
