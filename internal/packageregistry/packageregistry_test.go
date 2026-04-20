// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package packageregistry

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/lockedfile"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/require"
)

var hndlEnv = handlerenv.HandlerEnvironment{
	ConfigFolder: "./testdir",
}

var packageRegistry = CurrentPackageRegistry{"package1": &VMAppPackageCurrent{
	ApplicationName:    "package1",
	DirectDownloadOnly: false,
	InstallCommand:     "install_1.ps1",
	RemoveCommand:      "remove_1.ps1",
	UpdateCommand:      "update_1.ps1",
	Version:            "1.2.3.1",
}, "package2": &VMAppPackageCurrent{
	ApplicationName:    "package2",
	DirectDownloadOnly: true,
	InstallCommand:     "install_2.ps1",
	RemoveCommand:      "remove_2.ps1",
	UpdateCommand:      "update_2.ps1",
	Version:            "1.2.3.2",
}}

func nopLog() *logging.ExtensionLogger {
	return logging.New(nil)
}

func initializeTest(t *testing.T) {
	err := os.MkdirAll(hndlEnv.ConfigFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		os.Stderr.WriteString("could not create handler environment config directory")
		t.Fatal(err)
	}
}

func cleanupTest() {
	os.RemoveAll(hndlEnv.ConfigFolder)
}

func TestPackageRegistryReadWrite(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	var pkgHndlr IPackageRegistry
	pkgHndlr, err := New(nopLog(), &hndlEnv, time.Second)
	require.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(packageRegistry)
	require.NoError(t, err, "operation should not throw error")

	result, err := pkgHndlr.GetExistingPackages()
	require.NoError(t, err, "operation should not throw error")

	_, ok1 := result["package1"]
	_, ok2 := result["package2"]
	require.True(t, ok1, "key should be present in dictionary")
	require.True(t, ok2, "key should be present in dictionary")

	require.True(t, reflect.DeepEqual(packageRegistry, result), "the maps should be equal")

	// test overwrite
	pkg := packageRegistry["package1"]
	pkg.Version = "new version"
	packageRegistry["package1"] = pkg

	err = pkgHndlr.WriteToDisk(packageRegistry)
	require.NoError(t, err, "operation should not throw error")

	result, err = pkgHndlr.GetExistingPackages()
	require.NoError(t, err, "operation should not throw error")

	_, ok1 = result["package1"]
	_, ok2 = result["package2"]
	require.True(t, ok1, "key should be present in dictionary")
	require.True(t, ok2, "key should be present in dictionary")

	require.True(t, reflect.DeepEqual(packageRegistry, result), "the maps should be equal")
	err = pkgHndlr.Close()
	require.NoError(t, err, "operation should not throw error")
}

func TestPackageRegistryDeserialization_NumRebootsOccurred_DefaultToZero(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	var pkgHndlr IPackageRegistry
	pkgHndlr, err := New(nopLog(), &hndlEnv, time.Second)
	require.NoError(t, err, "operation should not throw error")

	// Overwrite package2
	pkg := packageRegistry["package2"]
	pkg.NumRebootsOccurred = 1
	packageRegistry["package2"] = pkg

	err = pkgHndlr.WriteToDisk(packageRegistry)
	require.NoError(t, err, "operation should not throw error")

	result, err := pkgHndlr.GetExistingPackages()
	require.NoError(t, err, "operation should not throw error")

	package1 := result["package1"]
	package2 := result["package2"]

	require.Equal(t, 0, package1.NumRebootsOccurred, "deserializing package from registry with no reboots property should default to 0")
	require.Equal(t, 1, package2.NumRebootsOccurred, "num reboots occurred for package2 should be 1")

	err = pkgHndlr.Close()
	require.NoError(t, err, "operation should not throw error")
}

func TestValuesAreProperlySaved(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	reg1 := CurrentPackageRegistry{"p1": &VMAppPackageCurrent{ApplicationName: "p1", Version: "1.1"}}
	var pkgHndlr IPackageRegistry
	pkgHndlr, err := New(nopLog(), &hndlEnv, time.Second)
	require.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(reg1)
	require.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.Close()
	require.NoError(t, err, "operation should not throw error")

	pkgHndlr, err = New(nopLog(), &hndlEnv, time.Second)
	require.NoError(t, err, "operation should not throw error")
	result, err := pkgHndlr.GetExistingPackages()
	require.True(t, reflect.DeepEqual(reg1, result), "the maps should be equal")
	err = pkgHndlr.Close()
	require.NoError(t, err, "operation should not throw error")

	// write different data again, test if it is consistent
	reg2 := CurrentPackageRegistry{"p2": &VMAppPackageCurrent{ApplicationName: "p2", Version: "2.1"}}
	pkgHndlr, err = New(nopLog(), &hndlEnv, time.Second)
	require.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.WriteToDisk(reg2)
	require.NoError(t, err, "operation should not throw error")
	err = pkgHndlr.Close()
	require.NoError(t, err, "operation should not throw error")

	pkgHndlr, err = New(nopLog(), &hndlEnv, time.Second)
	require.NoError(t, err, "operation should not throw error")
	result, err = pkgHndlr.GetExistingPackages()
	require.True(t, reflect.DeepEqual(reg2, result), "the maps should be equal")
	err = pkgHndlr.Close()
	require.NoError(t, err, "operation should not throw error")

}

func TestOnlyOneInstanceofPackageRegistryCanExist(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	pkgHndlr1, err := New(nopLog(), &hndlEnv, 60*time.Second)
	require.NoError(t, err, "operation should not throw error")
	pkgHndlr2, err := New(nopLog(), &hndlEnv, time.Second)
	require.Error(t, err, "operation should throw error")
	require.Nil(t, pkgHndlr2, "package handler instance should be nil")
	_, ok := err.(*lockedfile.FileLockTimeoutError)
	require.True(t, ok, "Error type should be FileLockTimeoutError")
	err = pkgHndlr1.Close()
	require.NoError(t, err, "operation should not throw error")

	// let things settle down before retrying
	time.Sleep(60 * time.Second)
	pkgHndlr2, err = New(nopLog(), &hndlEnv, time.Second)
	require.NoError(t, err, "operation should not throw error")
	err = pkgHndlr2.Close()
	require.NoError(t, err, "operation should not throw error")
}
