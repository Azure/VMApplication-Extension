// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"errors"

	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/Azure/azure-extension-platform/vmextension"
)

func getVMAppIncomingCollection(settings extdeserialization.VmAppProtectedSettings, communicator hostgacommunicator.IHostGaCommunicator, el *logging.ExtensionLogger) (packageregistry.VMAppPackageIncomingCollection, *vmextension.ErrorWithClarification) {

	incomingCollection := make(packageregistry.VMAppPackageIncomingCollection, 0)
	for _, app := range settings {
		if app.ApplicationName == "" {
			return nil, vmextension.NewErrorWithClarificationPtr(utils.DataFormat_MissingApplicationName, errors.New("missing application name"))
		}
		vmAppInfo, ewc := communicator.GetVMAppInfo(el, app.ApplicationName)
		if ewc != nil {
			// TODO: ignore errors?
			return incomingCollection, ewc
		}
		if vmAppInfo.Version == "" {
			return nil, vmextension.NewErrorWithClarificationPtr(utils.HGAP_InvalidAppInfo, errors.New("HostGA did not return a valid vmAppInfo"))
		}

		var applicationRebootBehavior packageregistry.RebootBehaviorEnum
		switch vmAppInfo.RebootBehavior {
		case packageregistry.None.ToString():
			applicationRebootBehavior = packageregistry.None
		case packageregistry.Rerun.ToString():
			applicationRebootBehavior = packageregistry.Rerun
		default:
			applicationRebootBehavior = packageregistry.None
		}

		incomingPackage := packageregistry.VMAppPackageIncoming{
			ApplicationName:                 app.ApplicationName,
			Order:                           app.Order,
			Version:                         vmAppInfo.Version,
			InstallCommand:                  vmAppInfo.InstallCommand,
			RemoveCommand:                   vmAppInfo.RemoveCommand,
			UpdateCommand:                   vmAppInfo.UpdateCommand,
			DirectDownloadOnly:              vmAppInfo.DirectDownloadOnly,
			ConfigExists:                    vmAppInfo.ConfigExists,
			ConfigFileName:                  vmAppInfo.ConfigFileName,
			PackageFileName:                 vmAppInfo.PackageFileName,
			TreatFailureAsDeploymentFailure: app.TreatFailureAsDeploymentFailure,
			RebootBehavior:                  applicationRebootBehavior,
		}
		incomingCollection = append(incomingCollection, &incomingPackage)
	}
	return incomingCollection, nil
}
