package main

import (
	"encoding/json"
	"errors"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/settings"
	"github.com/go-kit/kit/log"
)

type VmAppSetting struct {
	ApplicationName string `json:"applicationName"`
	Order           *int   `json:"order"`
}

type VmAppProtectedSettings []*VmAppSetting

func getVMAppIncomingCollection(settings *settings.HandlerSettings, communicator hostgacommunicator.IHostGaCommunicator, ctx log.Logger) (packageregistry.VMAppPackageIncomingCollection, error) {
	protSettings, err := getVMAppProtectedSettings(settings)
	if err != nil {
		return nil, err
	}
	incomingCollection := make(packageregistry.VMAppPackageIncomingCollection, 0)
	for _, app := range protSettings {
		if app.ApplicationName == "" {
			return nil, errors.New("Missing application name")
		}
		vmAppInfo, err := communicator.GetVMAppInfo(ctx, app.ApplicationName)
		if err != nil {
			// TODO: ignore errors?
			return incomingCollection, err
		}
		if vmAppInfo.Version == "" || vmAppInfo.Operation == "" {
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
