package packageregistry

import (
	"github.com/Azure/VMApplication-Extension/VmApp/constants"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/Azure/VMApplication-Extension/pkg/lockedfile"
	"github.com/stretchr/testify/assert"
	"os"
	"reflect"
	"testing"
	"time"
)

var hndlEnv = vmextensionhelper.HandlerEnvironment{
	ConfigFolder: "./testdir",
}

var packageRegistry = CurrentPackageRegistry{"package1": &VMAppPackageCurrent{
	ApplicationName:       "package1",
	ConfigurationLocation: "some configuration location1",
	DirectDownloadOnly:    false,
	InstallCommand:        "install_1.ps1",
	PackageLocation:       "some package location1",
	RemoveCommand:         "remove_1.ps1",
	UpdateCommand:         "update_1.ps1",
	Version:               "1.2.3.1",
}, "package2": &VMAppPackageCurrent{
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
	err := os.MkdirAll(hndlEnv.ConfigFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
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
	var pkgHndlr IPackageRegistry
	pkgHndlr, err := New(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(packageRegistry)
	assert.NoError(t, err, "operation should not throw error")

	result, err := pkgHndlr.GetExistingPackages()
	assert.NoError(t, err, "operation should not throw error")

	_, ok1 := result["package1"]
	_, ok2 := result["package2"]
	assert.True(t, ok1, "key should be present in dictionary")
	assert.True(t, ok2, "key should be present in dictionary")

	assert.True(t, reflect.DeepEqual(packageRegistry, result), "the maps should be equal")

	// test overwrite
	pkg := packageRegistry["package1"]
	pkg.Version = "new version"
	packageRegistry["package1"] = pkg


	err = pkgHndlr.WriteToDisk(packageRegistry)
	assert.NoError(t, err, "operation should not throw error")

	result, err = pkgHndlr.GetExistingPackages()
	assert.NoError(t, err, "operation should not throw error")

	_, ok1 = result["package1"]
	_, ok2 = result["package2"]
	assert.True(t, ok1, "key should be present in dictionary")
	assert.True(t, ok2, "key should be present in dictionary")

	assert.True(t, reflect.DeepEqual(packageRegistry, result), "the maps should be equal")
	err = pkgHndlr.Close()
	assert.NoError(t, err, "operation should not throw error")
}

func TestValuesAreProperlySaved(t *testing.T){
	initializeTest(t)
	defer cleanupTest()
	reg1 := CurrentPackageRegistry{"p1" : &VMAppPackageCurrent{ApplicationName: "p1", Version:"1.1"}}
	var pkgHndlr IPackageRegistry
	pkgHndlr, err := New(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(reg1)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.Close()
	assert.NoError(t, err, "operation should not throw error")

	pkgHndlr, err = New(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	result, err := pkgHndlr.GetExistingPackages()
	assert.True(t, reflect.DeepEqual(reg1, result), "the maps should be equal")
	err = pkgHndlr.Close()
	assert.NoError(t, err, "operation should not throw error")

	// write different data again, test if it is consistent
	reg2 := CurrentPackageRegistry{"p2" : &VMAppPackageCurrent{ApplicationName: "p2", Version:"2.1"}}
	pkgHndlr, err = New(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(reg2)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.Close()
	assert.NoError(t, err, "operation should not throw error")

	pkgHndlr, err = New(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	result, err = pkgHndlr.GetExistingPackages()
	assert.True(t, reflect.DeepEqual(reg2, result), "the maps should be equal")
	err = pkgHndlr.Close()
	assert.NoError(t, err, "operation should not throw error")

}

func TestOnlyOneInstanceofPackageHandlerCanExist(t *testing.T){
	initializeTest(t)
	defer cleanupTest()
	pkgHndlr1, err := New(&hndlEnv, 60 * time.Second)
	assert.NoError(t, err, "operation should not throw error")
	pkgHndlr2, err := New(&hndlEnv, time.Second)
	assert.Error(t, err, "operation should throw error")
	assert.Nil(t, pkgHndlr2, "package handler instance should be nil")
	_, ok := err.(*lockedfile.FileLockTimeoutError)
	assert.True(t, ok, "Error type should be FileLockTimeoutError")
	err = pkgHndlr1.Close()
	assert.NoError(t, err, "operation should not throw error")
	pkgHndlr2, err = New(&hndlEnv, time.Second)
	assert.NoError(t, err, "operation should not throw error")
	err = pkgHndlr2.Close()
	assert.NoError(t, err, "operation should not throw error")
}