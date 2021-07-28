package main

import (
	"errors"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)


func getVMAppIncomingCollection(settings VmAppProtectedSettings, communicator hostgacommunicator.IHostGaCommunicator, el *logging.ExtensionLogger) (packageregistry.VMAppPackageIncomingCollection, error) {

	incomingCollection := make(packageregistry.VMAppPackageIncomingCollection, 0)
	for _, app := range settings {
		if app.ApplicationName == "" {
			return nil, errors.New("Missing application name")
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

