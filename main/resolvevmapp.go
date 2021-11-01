package main

import (
	"encoding/json"
	"errors"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/Azure/azure-extension-platform/pkg/settings"
)

type VmAppSetting struct {
	ApplicationName string `json:"applicationName"`
	Order           *int   `json:"order"`
}

type VmAppProtectedSettings []*VmAppSetting

func getVMAppIncomingCollection(settings *settings.HandlerSettings, communicator hostgacommunicator.IHostGaCommunicator, el *logging.ExtensionLogger) (packageregistry.VMAppPackageIncomingCollection, error) {
	protSettings, err := getVMAppProtectedSettings(settings)
	if err != nil {
		return nil, err
	}
	incomingCollection := make(packageregistry.VMAppPackageIncomingCollection, 0)
	for _, app := range protSettings {
		if app.ApplicationName == "" {
			return nil, errors.New("missing application name")
		}
		vmAppInfo, err := communicator.GetVMAppInfo(el, app.ApplicationName)
		if err != nil {
			// TODO: ignore errors?
			return incomingCollection, err
		}
		if vmAppInfo.Version == "" {
			return nil, errors.New("HostGA did not return a valid vmAppInfo")
		}
		incomingPackage := packageregistry.VMAppPackageIncoming{
			ApplicationName:    app.ApplicationName,
			Order:              app.Order,
			Version:            vmAppInfo.Version,
			InstallCommand:     vmAppInfo.InstallCommand,
			RemoveCommand:      vmAppInfo.RemoveCommand,
			UpdateCommand:      vmAppInfo.UpdateCommand,
			DirectDownloadOnly: vmAppInfo.DirectDownloadOnly,
			ConfigExists:       vmAppInfo.ConfigExists,
			ConfigFileName:     vmAppInfo.ConfigFileName,
			PackageFileName:    vmAppInfo.PackageFileName,
		}
		incomingCollection = append(incomingCollection, &incomingPackage)
	}
	return incomingCollection, nil
}

func getVMAppProtectedSettings(settings *settings.HandlerSettings) (VmAppProtectedSettings, error) {
	vmAppProtectedSettings := VmAppProtectedSettings{}
	err := json.Unmarshal([]byte(settings.ProtectedSettings), &vmAppProtectedSettings)
	if err != nil {
		return nil, err
	}
	return vmAppProtectedSettings, err
}
