// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/environmentmanager"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/Azure/azure-extension-platform/pkg/seqno"
	"github.com/Azure/azure-extension-platform/pkg/settings"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/stretchr/testify/require"
)

var cannotCreatePackageRegistryError = "Could not create package registry: no such file or directory"

type mockEnvironmentManager struct {
	handlerEnv *handlerenv.HandlerEnvironment
}

// compile-time assertion to ensure mockEnvironmentManager satisfies IGetVMExtensionEnvironmentManager interface
var _ environmentmanager.IGetVMExtensionEnvironmentManager = (*mockEnvironmentManager)(nil)

func (m *mockEnvironmentManager) GetHandlerEnvironment(name string, version string) (*handlerenv.HandlerEnvironment, error) {
	return m.handlerEnv, nil
}

func (m *mockEnvironmentManager) FindSeqNum(el logging.ILogger, configFolder string) (uint, error) {
	return 0, nil
}

func (m *mockEnvironmentManager) GetCurrentSequenceNumber(el logging.ILogger, retriever seqno.ISequenceNumberRetriever, name, version string) (uint, error) {
	return 0, extensionerrors.ErrNoSettingsFiles
}

func (m *mockEnvironmentManager) GetHandlerSettings(el logging.ILogger, he *handlerenv.HandlerEnvironment) (*settings.HandlerSettings, error) {
	return nil, extensionerrors.ErrNoSettingsFiles
}

func (m *mockEnvironmentManager) SetSequenceNumberInternal(extensionName, extensionVersion string, seqNo uint) error {
	return nil
}

func Test_linuxUpdateInstallFlow_BackupAndRestoreDataFolderAndRegistry(t *testing.T) {
	environment := &mockEnvironmentManager{}
	oldGetVMExtension := buildExtensionFromInitInfoFunc
	buildExtensionFromInitInfoFunc = func(ii *vmextensionhelper.InitializationInfo) (*vmextensionhelper.VMExtension, error) {
		return vmextensionhelper.GetVMExtensionForTesting(ii, environment)
	}
	defer func() {
		buildExtensionFromInitInfoFunc = oldGetVMExtension
	}()

	originalExtensionName := ExtensionName
	ExtensionName = "TestExtension"
	defer func() {
		ExtensionName = originalExtensionName
	}()

	oldVersion := "0.0.1"
	originalExtensionVersion := ExtensionVersion

	root := t.TempDir()
	currentVersionDir := filepath.Join(root, ExtensionName+"-"+ExtensionVersion)
	oldVersionDir := filepath.Join(root, ExtensionName+"-"+oldVersion)

	// Create the version directories
	err := os.MkdirAll(currentVersionDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(oldVersionDir, 0755)
	require.NoError(t, err)

	testHandlerEnvironment := &handlerenv.HandlerEnvironment{
		HeartbeatFile: "heartbeat",
		StatusFolder:  "status",
		ConfigFolder:  filepath.Join(oldVersionDir, "config"),
		LogFolder:     filepath.Join(oldVersionDir, "log"),
		DataFolder:    filepath.Join(oldVersionDir, "data"),
		EventsFolder:  filepath.Join(oldVersionDir, "events"),
	}

	// Create required directories for both versions
	for _, dir := range []string{currentVersionDir, oldVersionDir} {
		err = os.MkdirAll(filepath.Join(dir, "status"), 0755)
		require.NoError(t, err)
		err = os.MkdirAll(filepath.Join(dir, "config"), 0755)
		require.NoError(t, err)
		err = os.MkdirAll(filepath.Join(dir, "log"), 0755)
		require.NoError(t, err)
		err = os.MkdirAll(filepath.Join(dir, "events"), 0755)
		require.NoError(t, err)
	}

	environment.handlerEnv = testHandlerEnvironment

	ExtensionVersion = oldVersion
	oldExtension, err := getVMExtension()
	require.NoError(t, err)
	ExtensionVersion = originalExtensionVersion

	environment.handlerEnv = &handlerenv.HandlerEnvironment{
		HeartbeatFile: "heartbeat",
		StatusFolder:  "status",
		ConfigFolder:  filepath.Join(currentVersionDir, "config"),
		LogFolder:     filepath.Join(currentVersionDir, "log"),
		DataFolder:    filepath.Join(currentVersionDir, "data"),
		EventsFolder:  filepath.Join(currentVersionDir, "events"),
	}

	currentExtension, err := getVMExtension()
	require.NoError(t, err)

	currentConfigDir := filepath.Join(currentVersionDir, "config")
	oldConfigDir := filepath.Join(oldVersionDir, "config")
	currentDataFolder := filepath.Join(currentVersionDir, "data")
	oldDataFolder := filepath.Join(oldVersionDir, "data")

	err = os.MkdirAll(currentConfigDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(oldConfigDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(currentDataFolder, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(oldDataFolder, 0755)
	require.NoError(t, err)

	// Seed current DataFolder with downloaded package content that must survive update/uninstall/install.
	payloadPath := filepath.Join(currentDataFolder, "downloads", "appA", "1.0", "payload.txt")
	err = os.MkdirAll(filepath.Dir(payloadPath), 0755)
	require.NoError(t, err)
	payloadContent := []byte("payload from current DataFolder")
	err = os.WriteFile(payloadPath, payloadContent, 0644)
	require.NoError(t, err)

	// Seed old DataFolder to validate old-uninstall behavior in this simulation.
	err = os.WriteFile(filepath.Join(oldDataFolder, "old-only.txt"), []byte("old version file"), 0644)
	require.NoError(t, err)

	// Create old-version package registry that points to the downloaded package location.
	oldPkr, err := packageregistry.New(oldExtension.ExtensionLogger, oldExtension.HandlerEnv, 1*time.Second)
	require.NoError(t, err)

	oldPackages := packageregistry.CurrentPackageRegistry{
		"appA": &packageregistry.VMAppPackageCurrent{
			ApplicationName: "appA",
			Version:         "1.0",
			DownloadDir:     filepath.Dir(payloadPath),
		},
	}
	err = oldPkr.WriteToDisk(oldPackages)
	require.NoError(t, err)
	err = oldPkr.Close()
	require.NoError(t, err)

	oldRegistryPath := filepath.Join(oldConfigDir, packageregistry.LocalApplicationRegistryFileName)
	oldRegistryContentBeforeUpdate, err := os.ReadFile(oldRegistryPath)
	require.NoError(t, err)

	// Set os.Args to ["update"] to simulate update command
	os.Args = []string{ExtensionName, "update"}
	currentExtension.Do()

	// validatr the following after update and before install:
	// 1) current registry should match old registry content from before update (i.e. registry was copied over by update callback)
	// 2) backup DataFolder should preserve downloaded package content (i.e. DataFolder was backed up by update callback)
	// 3) current DataFolder should be recreated as empty during update (i.e. original DataFolder was renamed to backup and a new empty DataFolder was created by platform before invoking update callback)
	currentRegistryPath := filepath.Join(currentConfigDir, packageregistry.LocalApplicationRegistryFileName)
	currentRegistryContent, err := os.ReadFile(currentRegistryPath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(oldRegistryContentBeforeUpdate, currentRegistryContent), "current registry should match old registry content from before update")

	backupDir := getDataFolderBackupPath(currentExtension)
	backupPayloadPath := filepath.Join(backupDir, "downloads", "appA", "1.0", "payload.txt")
	backupPayloadContent, err := os.ReadFile(backupPayloadPath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(payloadContent, backupPayloadContent), "backup DataFolder should preserve downloaded package content")

	currentDataEntries, err := os.ReadDir(currentDataFolder)
	require.NoError(t, err)
	require.Len(t, currentDataEntries, 0, "current DataFolder should be recreated as empty during update")

	// Run old version uninstall - set os.Args to ["uninstall"]
	os.Args = []string{ExtensionName, "uninstall"}
	oldExtension.Do()

	// Validate that backup DataFolder still exists after old version uninstall.
	// Old-version DataFolder may be removed as part of uninstall cleanup.
	_, err = os.Stat(backupDir)
	require.NoError(t, err)

	// Run current-version install - set os.Args to ["install"]
	os.Args = []string{ExtensionName, "install"}
	currentExtension.Do()
	// validate that backup DataFolder was merged back into current DataFolder and that original downloaded package content is present in the restored DataFolder

	_, err = os.Stat(backupDir)
	require.True(t, os.IsNotExist(err), "backup should be consumed by install restore")
	restoredPayload, err := os.ReadFile(payloadPath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(payloadContent, restoredPayload), "restored DataFolder should contain original downloaded package content")

	// Parse package registry from current version and validate package + on-disk package file.
	currentPkr, err := packageregistry.New(currentExtension.ExtensionLogger, currentExtension.HandlerEnv, 1*time.Second)
	require.NoError(t, err)
	defer currentPkr.Close()

	packages, err := currentPkr.GetExistingPackages()
	require.NoError(t, err)
	require.Len(t, packages, 1)
	appA, ok := packages["appA"]
	require.True(t, ok, "appA should exist in migrated package registry")
	require.Equal(t, filepath.Dir(payloadPath), appA.DownloadDir)

	payloadFromRegistryPath := filepath.Join(appA.DownloadDir, "payload.txt")
	payloadFromRegistry, err := os.ReadFile(payloadFromRegistryPath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(payloadContent, payloadFromRegistry), "file referenced by package registry should exist and match expected content")
}
