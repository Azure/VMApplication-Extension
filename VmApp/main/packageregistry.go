package main

import (
	"encoding/json"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"io/ioutil"
	"os"
	"path"
)

const (
	localApplicationRegistryFileName       = "applicationRegistry"
	localApplicationRegistryBackupFileName = "applicationRegistry.backup"
	activeChangesFileName                  = "applicationRegistry.active"
)

// defines a map between the application name and the other properties of the application
type PackageRegistry map [string]VMAppsPackage

type VMAppsPackages []VMAppsPackage

type VMAppsPackage struct {
	ApplicationName       string `json:"ApplicationName"`
	PackageLocation       string `json:"location"`
	ConfigurationLocation string `json:"config"`
	Version               string `json:"version"`
	InstallCommand        string `json:"install"`
	RemoveCommand         string `json:"remove"`
	UpdateCommand         string `json:"update"`
	DirectDownloadOnly    bool   `json:"directOnly"`
}


// returns a map of VMApps Name to VMAppsPackage for all packages that are currently installed on the VM
// do not call directly except for test, meant to be called from the wrapper function in packageregistry_linux or
// packageregistry_windows
func getExistingPackages(handlerEnv *vmextensionhelper.HandlerEnvironment)(PackageRegistry, error){
	appRegistryFilePath := path.Join(handlerEnv.ConfigFolder, localApplicationRegistryFileName)
	// make an empty byte slice with 4KB default size
	fileBytes, err := ioutil.ReadFile(appRegistryFilePath)
	if err != nil {
		return nil, err
	}

	vmAppsPackages := VMAppsPackages{}
	err = json.Unmarshal(fileBytes, &vmAppsPackages)
	if err != nil {
		return nil, err
	}

	retval := make(map [string]VMAppsPackage)

	for _, v := range vmAppsPackages{
		retval[v.ApplicationName] = v
	}

	return retval, nil
}

// do not call directly except for test, meant to be called from the wrapper function in packageregistry_linux or
// packageregistry_windows
func (self *PackageRegistry)writeToDisk(handlerEnv *vmextensionhelper.HandlerEnvironment)(error){
	values := make (VMAppsPackages, 0)
	for _, v := range (*self){
		values = append(values, v)
	}
	bytes, err := json.Marshal(values)
	if err != nil {
		return err
	}

	appRegistryFilePath := path.Join(handlerEnv.ConfigFolder, localApplicationRegistryFileName)
	file, err := os.OpenFile(appRegistryFilePath, os.O_WRONLY|os.O_CREATE, 0700)
	defer file.Close()
	if err != nil {
		return err
	}

	_, err = file.Write(bytes)
	if err != nil {
		return err
	}
	return nil
}


