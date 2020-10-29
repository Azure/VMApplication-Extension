package main

import (
	"encoding/json"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/azure-extension-platform/pkg/settings"
)

type VmAppSetting struct {
	ApplicationName string `json:"applicationName"`
	Order           *int   `json:"order"`
}

type VmAppProtectedSettings []*VmAppSetting

func getVMAppIncomingCollection(settings *settings.HandlerSettings, communicator hostgacommunicator.IHostGaCommunicator)(packageregistry.VMAppPackageIncomingCollection, error){
	protSettings, err := getVMAppProtectedSettings(settings)
	if err != nil {
		return nil, err
	}
	incomingCollection := make(packageregistry.VMAppPackageIncomingCollection, 0)
	for _, app := range protSettings{
		incomingPackage, err := communicator.GetVMAppInfo(app.ApplicationName)
		if err != nil {
			// TODO: ignore errors?
			return incomingCollection, err
		}
		incomingCollection = append(incomingCollection, incomingPackage)
	}
	return incomingCollection, nil
}

func getVMAppProtectedSettings (settings *settings.HandlerSettings)(VmAppProtectedSettings, error){
	bytes, err := json.Marshal(settings.ProtectedSettings)
	if err!= nil {
		return nil, err
	}

	vmAppProtectedSettings := VmAppProtectedSettings{}
	err = json.Unmarshal(bytes, vmAppProtectedSettings)
	if err != nil {
		return nil, err
	}
	return vmAppProtectedSettings, err
}


