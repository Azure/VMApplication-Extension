package main

import (
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/stretchr/testify/assert"
	"os"
	"reflect"
	"testing"
)

var hndlEnv = vmextensionhelper.HandlerEnvironment{
	ConfigFolder: "./testdir",
}

var packageRegistry = PackageRegistry{"package1": VMAppsPackage{
	ApplicationName:       "package1",
	ConfigurationLocation: "some configuration location1",
	DirectDownloadOnly:    false,
	InstallCommand:        "install_1.ps1",
	PackageLocation:       "some package location1",
	RemoveCommand:         "remove_1.ps1",
	UpdateCommand:         "update_1.ps1",
	Version:               "1.2.3.1",
}, "package2": VMAppsPackage{
	ApplicationName:       "package2",
	ConfigurationLocation: "some configuration location2",
	DirectDownloadOnly:    true,
	InstallCommand:        "install_2.ps1",
	PackageLocation:       "some package location2",
	RemoveCommand:         "remove_2.ps1",
	UpdateCommand:         "update_2.ps1",
	Version:               "1.2.3.2",
}}

func TestPackageRegistryReadWrite(t *testing.T){
	// initialzie
	err := os.MkdirAll(hndlEnv.ConfigFolder, 0700)
	if err != nil {
		os.Stderr.WriteString("could not create handler environment config directory")
		return
	}
	// cleanup
	defer os.RemoveAll(hndlEnv.ConfigFolder)
	err = packageRegistry.writeToDisk(&hndlEnv)
	assert.NoError(t, err, "operation should not throw error")
	result, err := getExistingPackages(&hndlEnv)
	assert.NoError(t, err, "operation should not throw error")

	_, ok1 := result["package1"]
	_, ok2 := result["package2"]
	assert.True(t, ok1, "key should be present in dictionary")
	assert.True(t, ok2, "key should be present in dictionary")

	assert.True(t, reflect.DeepEqual(packageRegistry, result), "the maps should be equal")
}
