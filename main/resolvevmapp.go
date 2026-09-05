// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"errors"
	"fmt"

	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)

func getVMAppIncomingCollection(settings extdeserialization.VmAppProtectedSettings, communicator hostgacommunicator.IHostGaCommunicator, el *logging.ExtensionLogger) (packageregistry.VMAppPackageIncomingCollection, error) {

	incomingCollection := make(packageregistry.VMAppPackageIncomingCollection, 0)
	for _, app := range settings {
		if app.ApplicationName == "" {
			return nil, errors.New("missing application name")
		}

		var vmAppInfo *hostgacommunicator.VMAppMetadata
		err := requesthelper.WithRetriesOnError(el, requesthelper.ActualSleep, func() error {
			var err error
			vmAppInfo, err = communicator.GetVMAppInfo(el, app.ApplicationName, app.Version)
			if err != nil {
				return err
			}
			if vmAppInfo.Version == "" {
				return errors.New("HostGA did not return a valid vmAppInfo")
			}
			if vmAppInfo.Version != app.Version {
				return requesthelper.NewRetryableError(fmt.Errorf("HostGA returned version %q for application %q; expected %q", vmAppInfo.Version, app.ApplicationName, app.Version))
			}
			return nil
		})
		if err != nil {
			return incomingCollection, err
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
