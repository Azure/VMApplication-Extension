package main

import (
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/stretchr/testify/assert"
	"os"
	"reflect"
	"testing"
	"time"
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

func initializeTest(t *testing.T){
	err := os.MkdirAll(hndlEnv.ConfigFolder, 0700)
	if err != nil {
		os.Stderr.WriteString("could not create handler environment config directory")
		t.Fatal(err)
	}
}

func cleanupTest(){
	os.RemoveAll(hndlEnv.ConfigFolder)
}

func TestPackageRegistryReadWrite(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	var pkgHndlr IPackageHandler
	pkgHndlr, err := PackageHandlerInit(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(&packageRegistry)

	result, err := pkgHndlr.GetExistingPackages()
	assert.NoError(t, err, "operation should not throw error")

	_, ok1 := result["package1"]
	_, ok2 := result["package2"]
	assert.True(t, ok1, "key should be present in dictionary")
	assert.True(t, ok2, "key should be present in dictionary")

	assert.True(t, reflect.DeepEqual(packageRegistry, result), "the maps should be equal")
}

func TestOnlyOneInstanceofPackageHandlerCanExist(t *testing.T){
	initializeTest(t)
	defer cleanupTest()
	pkgHndlr1, err := PackageHandlerInit(&hndlEnv, 60 * time.Second)
	assert.NoError(t, err, "operation should not throw error")
	pkgHndlr2, err := PackageHandlerInit(&hndlEnv, time.Second)
	assert.Error(t, err, "operation should throw error")
	assert.Nil(t, pkgHndlr2, "package handler instnace should be nill")
	_, ok := err.(*FileLockTimeoutError)
	assert.True(t, ok, "Error type should ne FileLockTimeoutError")
	pkgHndlr1.Close()
	pkgHndlr2, err = PackageHandlerInit(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	pkgHndlr2.Close()
}