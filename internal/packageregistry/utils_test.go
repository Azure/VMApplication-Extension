package packageregistry

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestVMAppPackageIncomingToVmAppPackageCurrent(t *testing.T) {
	incoming := &VMAppPackageIncoming{
		ApplicationName: "TestApplication",
		Version: "3.0.1",
		PackageFileName: "installer_executable",
		ConfigFileName: "configFile",
		InstallCommand: "invoke_install --install",
		RemoveCommand: "invoke_remove --remove",
		UpdateCommand: "invoke_update --update",
		ConfigExists: true,
		DirectDownloadOnly: false,
		Order: nil,
	}
	current := VMAppPackageIncomingToVmAppPackageCurrent(incoming)
	assert.Equal(t, incoming.ApplicationName, current.ApplicationName)
	assert.Equal(t, incoming.Version, current.Version)
	assert.Equal(t, incoming.PackageFileName, current.PackageFileName)
	assert.Equal(t, incoming.ConfigFileName, current.ConfigFileName)
	assert.Equal(t, incoming.InstallCommand, current.InstallCommand)
	assert.Equal(t, incoming.RemoveCommand, current.RemoveCommand)
	assert.Equal(t, incoming.UpdateCommand, current.UpdateCommand)
	assert.Equal(t, incoming.ConfigExists, current.ConfigExists)
	assert.Equal(t, incoming.DirectDownloadOnly, current.DirectDownloadOnly)


	incoming.PackageFileName = ""
	incoming.ConfigFileName = ""
	current = VMAppPackageIncomingToVmAppPackageCurrent(incoming)
	assert.Equal(t, incoming.ApplicationName, current.PackageFileName)
	assert.Equal(t, incoming.ApplicationName + defaultConfigFileNameSuffix, current.ConfigFileName)

}
