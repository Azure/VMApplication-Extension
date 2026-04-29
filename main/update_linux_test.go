// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	vmextension "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/stretchr/testify/require"
)

// createTestFilesLinux creates a directory structure simulating multiple extension versions on Linux:
//
//	folderPath/ExtensionName-1.0.1/configFolderName/fileName (badcontent)
//	folderPath/ExtensionName-1.0.3/configFolderName/fileName (badcontent)
//	folderPath/ExtensionName-0.0.1/configFolderName/fileName (Test File Contents) — most recently modified
func createTestFilesLinux(folderPath, configFolderName, fileName string) error {
	for _, ver := range []string{"1.0.1", "0.0.1", "1.0.3"} {
		dirName := ExtensionName + "-" + ver
		err := os.MkdirAll(filepath.Join(folderPath, dirName, configFolderName), 0755)
		if err != nil {
			return err
		}
	}

	testContent := []byte("badcontent")
	err := os.WriteFile(filepath.Join(folderPath, ExtensionName+"-1.0.1", configFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(folderPath, ExtensionName+"-1.0.3", configFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}
	testContent = []byte("Test File Contents")
	time.Sleep(time.Second)
	err = os.WriteFile(filepath.Join(folderPath, ExtensionName+"-0.0.1", configFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}

	return nil
}

func Test_noInfiniteLoops(t *testing.T) {
	ExtensionName = "TestExtension"
	defer func() { ExtensionName = "" }()

	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}
	ext := createTestVMExtension(t, vmApplications)

	// this overwrite creates a path that does not contain a version folder, so the update function should return an error instead of infinitely looping
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, "someRandomFolder", "random2", "random3", "config")

	//call update
	err := vmAppUpdateCallback(ext)
	require.ErrorIs(t, err, errorExtensionVersionDirNotFound)
}

func Test_findVersionDir_fallsBackThroughComparisonFunctions(t *testing.T) {
	ExtensionName = "TestExtension"
	extensionVersionOriginalValue := ExtensionVersion

	defer func() {
		// revert it to what the other tests might expect after this test is run
		ExtensionVersion = "1.0.10"
		ExtensionName = ""
	}()

	// Create a directory structure: <root>/ExtensionName-1.0.10/config
	root := t.TempDir()
	expectedVersionedDirName := ExtensionName + "-" + extensionVersionOriginalValue
	versionDir := filepath.Join(root, expectedVersionedDirName, "config")
	err := os.MkdirAll(versionDir, 0755)
	require.NoError(t, err)

	// Subtest 1: env vars not set — falls back to ExtensionVersion match
	t.Run("no_env_vars_uses_ExtensionVersion_match", func(t *testing.T) {
		os.Unsetenv(string(vmextension.GuestAgentEnvVarExtensionVersion))
		os.Unsetenv(string(vmextension.GuestAgentEnvVarUpdateToVersion))

		parent, dirWithVersion, relPath, err := splitPathAroundVersionedDirLinux(versionDir)
		require.NoError(t, err)
		require.Equal(t, root, parent)
		require.Equal(t, "config", relPath)
		require.Equal(t, expectedVersionedDirName, dirWithVersion)
	})

	// Subtest 2: AZURE_GUEST_AGENT_EXTENSION_VERSION matches — uses first checker
	t.Run("extension_version_env_var_matches", func(t *testing.T) {
		t.Setenv(string(vmextension.GuestAgentEnvVarExtensionVersion), extensionVersionOriginalValue)
		os.Unsetenv(string(vmextension.GuestAgentEnvVarUpdateToVersion))

		parent, dirWithVersion, relPath, err := splitPathAroundVersionedDirLinux(versionDir)
		require.NoError(t, err)
		require.Equal(t, root, parent)
		require.Equal(t, "config", relPath)
		extensionVersionFromEnv, err := vmextension.GetGuestAgentEnvironmetVariable(vmextension.GuestAgentEnvVarExtensionVersion)
		require.NoError(t, err)
		require.Equal(t, ExtensionName+"-"+extensionVersionFromEnv, dirWithVersion)
	})

	// Subtest 3: VERSION env var matches — uses second checker
	t.Run("update_to_version_env_var_matches", func(t *testing.T) {
		os.Unsetenv(string(vmextension.GuestAgentEnvVarExtensionVersion))
		t.Setenv(string(vmextension.GuestAgentEnvVarUpdateToVersion), extensionVersionOriginalValue)

		parent, dirWithVersion, relPath, err := splitPathAroundVersionedDirLinux(versionDir)
		require.NoError(t, err)
		require.Equal(t, root, parent)
		require.Equal(t, "config", relPath)
		updateToVersionFromEnv, err := vmextension.GetGuestAgentEnvironmetVariable(vmextension.GuestAgentEnvVarUpdateToVersion)
		require.NoError(t, err)
		require.Equal(t, ExtensionName+"-"+updateToVersionFromEnv, dirWithVersion)
	})

	// Subtest 4: env vars set to wrong values — falls back to ExtensionVersion match
	t.Run("env_vars_wrong_falls_back_to_ExtensionVersion", func(t *testing.T) {
		t.Setenv(string(vmextension.GuestAgentEnvVarExtensionVersion), "9.9.9")
		t.Setenv(string(vmextension.GuestAgentEnvVarUpdateToVersion), "8.8.8")

		parent, dirWithVersion, relPath, err := splitPathAroundVersionedDirLinux(versionDir)
		require.NoError(t, err)
		require.Equal(t, root, parent)
		require.Equal(t, "config", relPath)
		require.Equal(t, expectedVersionedDirName, dirWithVersion)
	})

	// Subtest 5: env vars set to wrong values, ExtensionVersion doesn't match — falls back to pattern match
	t.Run("env_vars_wrong_falls_back_to_pattern", func(t *testing.T) {
		t.Setenv(string(vmextension.GuestAgentEnvVarExtensionVersion), "9.9.9")
		t.Setenv(string(vmextension.GuestAgentEnvVarUpdateToVersion), "8.8.8")

		ExtensionVersion = "1.0.0" // the directory was created with extension version 1.0.10, this should fail to match

		parent, dirWithVersion, relPath, err := splitPathAroundVersionedDirLinux(versionDir)
		require.NoError(t, err)
		require.Equal(t, root, parent)
		require.Equal(t, "config", relPath)
		require.Equal(t, expectedVersionedDirName, dirWithVersion) // should still find the version dir based on pattern match even though env vars and ExtensionVersion value don't match
	})

	// Subtest 6: no version dir in path and no env vars — should return error
	t.Run("no_version_dir_returns_error", func(t *testing.T) {
		os.Unsetenv(string(vmextension.GuestAgentEnvVarExtensionVersion))
		os.Unsetenv(string(vmextension.GuestAgentEnvVarUpdateToVersion))

		noVersionPath := filepath.Join(t.TempDir(), "noVersion", "data")
		err := os.MkdirAll(noVersionPath, 0755)
		require.NoError(t, err)

		_, _, _, err = splitPathAroundVersionedDirLinux(noVersionPath)
		require.ErrorIs(t, err, errorExtensionVersionDirNotFound)
	})
}

func Test_cannotFindPackageConfigFile(t *testing.T) {
	ExtensionName = "TestExtension"
	defer func() { ExtensionName = "" }()

	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}
	ext := createTestVMExtension(t, vmApplications)

	//set up test files
	configFolderName := "config"
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, ExtensionName+"-"+ExtensionVersion, configFolderName)

	//call update
	err := vmAppUpdateCallback(ext)
	require.ErrorIs(t, err, errorNoOlderPackageRegistryFileFound)
}

func Test_existingPackageRegistryFileIsNotOverwritten(t *testing.T) {
	ExtensionName = "TestExtension"
	defer func() { ExtensionName = "" }()

	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})

	configFolderName := "config"
	testFolderPath := t.TempDir()
	ext.HandlerEnv.ConfigFolder = filepath.Join(testFolderPath, ExtensionName+"-"+ExtensionVersion, configFolderName)
	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, 0755)
	require.NoError(t, err)
	fileName := packageregistry.LocalApplicationRegistryFileName
	err = createTestFilesLinux(testFolderPath, configFolderName, fileName)
	require.NoError(t, err)

	fileBytes := []byte("special message")
	packageRegistryFilePath := path.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	err = os.WriteFile(packageRegistryFilePath, fileBytes, 0777)
	require.NoError(t, err)
	err = vmAppUpdateCallback(ext)
	require.NoError(t, err)
	// verify file was not overwritten
	readBytes, err := os.ReadFile(packageRegistryFilePath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(fileBytes, readBytes))
}

func Test_vmAppUpdateCallback_endToEnd(t *testing.T) {
	ExtensionName = "TestExtension"
	defer func() { ExtensionName = "" }()

	runAndValidate := func(t *testing.T, createConfigDirBeforeUpdate bool) {
		t.Helper()

		ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})

		// --- Set up config folder structure with multiple old versions ---
		configRoot := t.TempDir()
		configFolderName := "config"
		currentConfigDir := filepath.Join(configRoot, ExtensionName+"-"+ExtensionVersion, configFolderName)
		if createConfigDirBeforeUpdate {
			err := os.MkdirAll(currentConfigDir, 0755)
			require.NoError(t, err)
		}
		ext.HandlerEnv.ConfigFolder = currentConfigDir

		// Creates ExtensionName-{1.0.1,0.0.1,1.0.3}/config/ with placeholder files;
		// 0.0.1 is written last so it has the most recent modification time
		fileName := packageregistry.LocalApplicationRegistryFileName
		err := createTestFilesLinux(configRoot, configFolderName, fileName)
		require.NoError(t, err)

		// Overwrite 1.0.3's registry file with real package data using the package registry API.
		// WriteToDisk updates the file's modification time, making 1.0.3 the most recently updated.
		mostRecentOldConfigDir := filepath.Join(configRoot, ExtensionName+"-1.0.3", configFolderName)
		ext.HandlerEnv.ConfigFolder = mostRecentOldConfigDir
		oldPkr, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, 1*time.Second)
		require.NoError(t, err)

		oldPackages := packageregistry.CurrentPackageRegistry{
			"appA": &packageregistry.VMAppPackageCurrent{
				ApplicationName: "appA",
				Version:         "1.0",
				DownloadDir:     "/var/lib/waagent/downloads/appA/1.0",
			},
			"appB": &packageregistry.VMAppPackageCurrent{
				ApplicationName: "appB",
				Version:         "2.0",
				DownloadDir:     "/var/lib/waagent/downloads/appB/2.0",
			},
		}
		time.Sleep(time.Second) // sleep before writing to ensure this file has the most recent mod time
		err = oldPkr.WriteToDisk(oldPackages)
		require.NoError(t, err)
		err = oldPkr.Close()
		require.NoError(t, err)

		// Read back the old registry content before update
		oldRegistryPath := filepath.Join(mostRecentOldConfigDir, packageregistry.LocalApplicationRegistryFileName)
		oldFileContents, err := os.ReadFile(oldRegistryPath)
		require.NoError(t, err)

		// Restore ConfigFolder to the current version
		ext.HandlerEnv.ConfigFolder = currentConfigDir

		// --- Call vmAppUpdateCallback ---
		err = vmAppUpdateCallback(ext)
		require.NoError(t, err)

		// --- Validation 1: Package registry file was copied from the old version ---
		newFileContents, err := os.ReadFile(filepath.Join(currentConfigDir, packageregistry.LocalApplicationRegistryFileName))
		require.NoError(t, err)
		require.True(t, bytes.Equal(oldFileContents, newFileContents),
			"package registry content should be copied from the old version")

		// --- Validation 2: Old version's registry should be emptied ---
		oldFileContentsAfterUpdate, err := os.ReadFile(oldRegistryPath)
		require.NoError(t, err)
		require.True(t, bytes.Equal([]byte("[]"), oldFileContentsAfterUpdate),
			"old version's package registry should be overwritten with empty list")

		// --- Validation 3: Copied registry should be readable and contain the expected packages ---
		pkr, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, 1*time.Second)
		require.NoError(t, err)
		defer pkr.Close()

		packages, err := pkr.GetExistingPackages()
		require.NoError(t, err)
		require.Len(t, packages, len(oldPackages))
		for name, expected := range oldPackages {
			actual, ok := packages[name]
			require.True(t, ok, "expected package %s not found in copied registry", name)
			require.Equal(t, expected.Version, actual.Version)
			require.Equal(t, expected.DownloadDir, actual.DownloadDir)
			require.Equal(t, expected.ApplicationName, actual.ApplicationName)
		}
	}

	t.Run("config_folder_already_exists", func(t *testing.T) {
		runAndValidate(t, true)
	})

	t.Run("config_folder_does_not_exist", func(t *testing.T) {
		runAndValidate(t, false)
	})
}

func Test_getDirNameCheckerWithExtensionVersionPattern_matchesWithPublisherPrefix(t *testing.T) {
	ExtensionName = "VMApplicationManagerLinux"
	actualDirName := "Microsoft.CPlat.Core.Edp.VMApplicationManagerLinux-1.0.18"
	checkerFunc := getDirNameCheckerWithExtensionVersionPattern()
	require.True(t, checkerFunc(actualDirName))
}

func Test_canFindOlderPackageRegistryFile(t *testing.T) {
	// test that case insensitivity works when looking for older package registry files
	unmodifiedExtensionName := ExtensionName
	ExtensionName = "Microsoft.CPlat.Core.EDP.VMApplicationManagerLinux"
	defer func() {
		ExtensionName = unmodifiedExtensionName
	}()

	testRoot := t.TempDir()
	oldConfigDir := filepath.Join(testRoot, "/var/lib/waagent/Microsoft.CPlat.Core.Edp.VMApplicationManagerLinux-1.0.18/config")
	oldPackageRegistryFilePath := filepath.Join(oldConfigDir, packageregistry.LocalApplicationRegistryFileName)
	err := os.MkdirAll(oldConfigDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(oldPackageRegistryFilePath, []byte("[]"), 0o644)
	require.NoError(t, err)
	head, target, tail, err := splitPathAroundVersionedDirLinux(oldConfigDir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(testRoot, "/var/lib/waagent"), head)
	require.Equal(t, "Microsoft.CPlat.Core.Edp.VMApplicationManagerLinux-1.0.18", target)
	require.Equal(t, "config", tail)
	packageRegistryFilePath, err := getMostRecentlyUpdatedPackageRegistryFile(head, tail, getDirNameCheckerWithExtensionVersionPattern())
	require.NoError(t, err)
	require.Equal(t, oldPackageRegistryFilePath, packageRegistryFilePath, "should be able to find older package registry file")
}
