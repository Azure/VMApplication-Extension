// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"bytes"
	"io/ioutil"
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

func Test_didFileMove(t *testing.T) {
	//set up test VM
	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}
	ext := createTestVMExtension(t, vmApplications)

	//set up test files
	runtimeFolderName := "RuntimeSettings"
	testFolderPath := ext.HandlerEnv.ConfigFolder                                                                 //path to create test version folders
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, ExtensionVersion, runtimeFolderName) //overwrite to match path pattern of config folder in VM
	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)                                                   //creates new folders
	require.NoError(t, err)
	fileName := packageregistry.LocalApplicationRegistryFileName //gets name of application registry file
	err = createTestFiles(testFolderPath, runtimeFolderName, fileName)
	require.NoError(t, err)
	// cleanup
	defer os.RemoveAll(testFolderPath)

	oldFileContents, err := os.ReadFile(filepath.Join(testFolderPath, "0.0.1", runtimeFolderName, fileName))
	require.NoError(t, err)

	//call update
	err = vmAppUpdateCallback(ext)
	require.NoError(t, err)

	oldFileContentsAfterUpdate, err := os.ReadFile(filepath.Join(testFolderPath, "0.0.1", runtimeFolderName, fileName))
	require.NoError(t, err)

	newFileContents, err := os.ReadFile(filepath.Join(ext.HandlerEnv.ConfigFolder, fileName))
	require.NoError(t, err)

	//checks
	require.True(t, bytes.Equal(oldFileContents, newFileContents))
	require.True(t, bytes.Equal([]byte("[]"), oldFileContentsAfterUpdate))
}

func Test_noInfiniteLoops(t *testing.T) {
	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}
	ext := createTestVMExtension(t, vmApplications)

	// this overwrite creates a path that does not contain a version folder, so the update function should return an error instead of infinitely looping
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, "someRadomFolder", "random2", "random3", "RuntimeSettings")

	//call update
	err := vmAppUpdateCallback(ext)
	require.ErrorIs(t, err, errorExtensionVersionDirNotFound)
}

func Test_cannotFindPackageConfigFile(t *testing.T) {
	order := 1
	vmApplications := []extdeserialization.VmAppSetting{
		{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}
	ext := createTestVMExtension(t, vmApplications)

	//set up test files
	runtimeFolderName := "RuntimeSettings"                                                                        //path to create test version folders
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, ExtensionVersion, runtimeFolderName) //overwrite to match path pattern of config folder in VM

	//call update
	err := vmAppUpdateCallback(ext)
	require.ErrorIs(t, err, errorNoOlderPakcageRegistryFileFound)
}

func Test_existingPackageRegistryFileIsNotOverwritten(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})

	runtimeFolderName := "RuntimeSettings"
	testFolderPath := ext.HandlerEnv.ConfigFolder                                                                 //path to create test version folders
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, ExtensionVersion, runtimeFolderName) //overwrite to match path pattern of config folder in VM
	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)                                                   //creates new folders
	require.NoError(t, err)
	fileName := packageregistry.LocalApplicationRegistryFileName //gets name of application registry file
	err = createTestFiles(testFolderPath, runtimeFolderName, fileName)
	require.NoError(t, err)
	// cleanup
	defer os.RemoveAll(testFolderPath)

	fileBytes := []byte("special message")
	packageRegistryFilePath := path.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	err = ioutil.WriteFile(packageRegistryFilePath, fileBytes, 0777)
	require.NoError(t, err)
	err = vmAppUpdateCallback(ext)
	require.NoError(t, err)
	// verify file was not overwritten
	readBytes, err := ioutil.ReadFile(packageRegistryFilePath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(fileBytes, readBytes))
}

func createTestFiles(folderPath, runtimeFolderName, fileName string) error {
	//create test directories
	err := os.MkdirAll(filepath.Join(folderPath, "1.0.1", runtimeFolderName), os.ModeDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "0.0.1", runtimeFolderName), os.ModeDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "1.0.3", runtimeFolderName), os.ModeDir)
	if err != nil {
		return err
	}

	//creating test file
	testContent := []byte("badcontent")
	err = os.WriteFile(filepath.Join(folderPath, "1.0.1", runtimeFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(folderPath, "1.0.3", runtimeFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}
	testContent = []byte("Test File Contents")
	time.Sleep(time.Second)
	err = os.WriteFile(filepath.Join(folderPath, "0.0.1", runtimeFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}

	return nil
}

// setupDataFolderForMoveTest creates a directory structure simulating older version data folders:
//
//	rootDir/<oldVersion>/downloads/appA/file.txt
//	rootDir/<oldVersion>/downloads/appB/file.txt
//
// and sets ext.HandlerEnv.DataFolder to rootDir/<ExtensionVersion>/downloads
func setupDataFolderForMoveTest(t *testing.T, ext *vmextension.VMExtension, oldVersions []string) string {
	t.Helper()
	rootDir := t.TempDir()
	downloadsSubpath := "downloads"

	// Create data folder for current version (empty)
	currentDataFolder := filepath.Join(rootDir, ExtensionVersion, downloadsSubpath)
	err := os.MkdirAll(currentDataFolder, os.ModeDir)
	require.NoError(t, err)
	ext.HandlerEnv.DataFolder = currentDataFolder

	// Create old version data folders with sample subdirectories and files
	for _, ver := range oldVersions {
		for _, app := range []string{"appA", "appB"} {
			appDir := filepath.Join(rootDir, ver, downloadsSubpath, app)
			err := os.MkdirAll(appDir, os.ModeDir)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(appDir, "file.txt"), []byte("content-"+ver+"-"+app), 0666)
			require.NoError(t, err)
		}
	}

	return rootDir
}

func Test_moveDownloadDirToCurrentVersion_copiesFromOlderVersions(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})
	rootDir := setupDataFolderForMoveTest(t, ext, []string{"0.0.1", "1.0.3"})
	defer os.RemoveAll(rootDir)

	// Ensure config folder exists for the package registry lock file
	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)
	require.NoError(t, err)

	err = moveDownloadDirToCurrentVersion(ext)
	require.NoError(t, err)

	// Verify subdirectories were copied into the current DataFolder
	for _, app := range []string{"appA", "appB"} {
		copiedFile := filepath.Join(ext.HandlerEnv.DataFolder, app, "file.txt")
		_, err := os.Stat(copiedFile)
		require.NoError(t, err, "expected copied file at %s", copiedFile)
	}
}

func Test_moveDownloadDirToCurrentVersion_skipsCurrentVersion(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})
	// Only create a data folder for the current version itself (no older versions)
	rootDir := t.TempDir()
	downloadsSubpath := "downloads"

	currentDataFolder := filepath.Join(rootDir, ExtensionVersion, downloadsSubpath)
	err := os.MkdirAll(currentDataFolder, os.ModeDir)
	require.NoError(t, err)
	ext.HandlerEnv.DataFolder = currentDataFolder

	// Create a subdirectory in the current version's data folder with a marker file
	appDir := filepath.Join(rootDir, ExtensionVersion, downloadsSubpath, "appFromCurrent")
	err = os.MkdirAll(appDir, os.ModeDir)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(appDir, "marker.txt"), []byte("should-not-be-copied"), 0666)
	require.NoError(t, err)

	err = os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)
	require.NoError(t, err)

	err = moveDownloadDirToCurrentVersion(ext)
	require.NoError(t, err)

	// The current version's own dirs should not be re-copied into DataFolder root
	// (DataFolder already IS the current version folder, so we just check no error)
}

func Test_moveDownloadDirToCurrentVersion_noOlderVersions(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})
	rootDir := t.TempDir()
	downloadsSubpath := "downloads"

	currentDataFolder := filepath.Join(rootDir, ExtensionVersion, downloadsSubpath)
	err := os.MkdirAll(currentDataFolder, os.ModeDir)
	require.NoError(t, err)
	ext.HandlerEnv.DataFolder = currentDataFolder

	err = os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)
	require.NoError(t, err)

	err = moveDownloadDirToCurrentVersion(ext)
	require.NoError(t, err)
}

func Test_moveDownloadDirToCurrentVersion_noVersionDirFound(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})
	// Set DataFolder to a path containing no version-pattern directory
	ext.HandlerEnv.DataFolder = filepath.Join(t.TempDir(), "noVersionHere", "data")
	err := os.MkdirAll(ext.HandlerEnv.DataFolder, os.ModeDir)
	require.NoError(t, err)

	err = moveDownloadDirToCurrentVersion(ext)
	require.ErrorIs(t, err, errorExtensionVersionDirNotFound)
}

func Test_moveDownloadDirToCurrentVersion_nonVersionDirsIgnored(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})
	rootDir := setupDataFolderForMoveTest(t, ext, []string{"0.0.1"})
	defer os.RemoveAll(rootDir)

	// Create a non-version directory sibling (should be ignored)
	nonVersionDir := filepath.Join(rootDir, "notAVersion", "downloads", "appX")
	err := os.MkdirAll(nonVersionDir, os.ModeDir)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(nonVersionDir, "file.txt"), []byte("should-not-copy"), 0666)
	require.NoError(t, err)

	err = os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)
	require.NoError(t, err)

	err = moveDownloadDirToCurrentVersion(ext)
	require.NoError(t, err)

	// Verify the non-version directory content was NOT copied
	_, err = os.Stat(filepath.Join(ext.HandlerEnv.DataFolder, "appX"))
	require.True(t, os.IsNotExist(err), "non-version directory content should not be copied")

	// Verify old version content WAS copied
	_, err = os.Stat(filepath.Join(ext.HandlerEnv.DataFolder, "appA", "file.txt"))
	require.NoError(t, err, "old version content should be copied")
}

func Test_moveAndUpdateDownloadDir_updatesRegistryPaths(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})
	oldVersion := "0.0.1"
	rootDir := setupDataFolderForMoveTest(t, ext, []string{oldVersion})
	defer os.RemoveAll(rootDir)

	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)
	require.NoError(t, err)

	// Write a package registry file with DownloadDir pointing to the old version
	oldDownloadDir := filepath.Join(rootDir, oldVersion, "downloads")
	registryContent := `[{"applicationName":"appA","version":"1.0","downloadDir":"` + filepath.ToSlash(oldDownloadDir) + `"},{"applicationName":"appB","version":"2.0","downloadDir":"` + filepath.ToSlash(oldDownloadDir) + `"}]`
	registryFilePath := filepath.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	err = os.WriteFile(registryFilePath, []byte(registryContent), 0666)
	require.NoError(t, err)

	// Move download dirs, then update paths in registry
	err = moveDownloadDirToCurrentVersion(ext)
	require.NoError(t, err)

	err = updateDonwnloadDirInPackageRegistryFile(ext)
	require.NoError(t, err)

	// Read back the registry and verify DownloadDir was updated to the current version
	pkr, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, 1*time.Second)
	require.NoError(t, err)
	defer pkr.Close()

	packages, err := pkr.GetExistingPackages()
	require.NoError(t, err)

	expectedDownloadDir := filepath.Join(rootDir, ExtensionVersion, "downloads")
	for _, pkg := range packages {
		require.Equal(t, expectedDownloadDir, pkg.DownloadDir,
			"DownloadDir for %s should point to current version", pkg.ApplicationName)
	}
}

func Test_updateDownloadDirInPackageRegistryFile_noPackages(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})

	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)
	require.NoError(t, err)

	// Write an empty package registry
	registryFilePath := filepath.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	err = os.WriteFile(registryFilePath, []byte("[]"), 0666)
	require.NoError(t, err)

	err = updateDonwnloadDirInPackageRegistryFile(ext)
	require.NoError(t, err)
}

func Test_updateDownloadDirInPackageRegistryFile_packageWithNoVersionInPath(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})

	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)
	require.NoError(t, err)

	// Set up DataFolder with a version path so findVersionDir succeeds
	dataRoot := t.TempDir()
	ext.HandlerEnv.DataFolder = filepath.Join(dataRoot, ExtensionVersion, "downloads")
	err = os.MkdirAll(ext.HandlerEnv.DataFolder, os.ModeDir)
	require.NoError(t, err)

	// Write a registry where DownloadDir has no version segment — regex won't match, so it warns but doesn't fail
	registryContent := `[{"applicationName":"appX","version":"1.0","downloadDir":"C:/noVersionHere/downloads"}]`
	registryFilePath := filepath.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	err = os.WriteFile(registryFilePath, []byte(registryContent), 0666)
	require.NoError(t, err)

	err = updateDonwnloadDirInPackageRegistryFile(ext)
	require.NoError(t, err)

	// Verify the DownloadDir is unchanged since no version dir was found
	pkr, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, 1*time.Second)
	require.NoError(t, err)
	defer pkr.Close()

	packages, err := pkr.GetExistingPackages()
	require.NoError(t, err)
	require.Len(t, packages, 1)
	require.Equal(t, "C:/noVersionHere/downloads", packages["appX"].DownloadDir,
		"DownloadDir should remain unchanged when no version dir is found")
}

func Test_vmAppUpdateCallback_endToEnd(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})
	oldVersion := "0.0.1"
	configSubpath := "RuntimeSettings"

	// --- Set up config folder structure: <root>/<extensionVersion>/RuntimeSettings ---
	configRoot := t.TempDir()
	oldConfigDir := filepath.Join(configRoot, oldVersion, configSubpath)
	err := os.MkdirAll(oldConfigDir, os.ModeDir)
	require.NoError(t, err)

	currentConfigDir := filepath.Join(configRoot, ExtensionVersion, configSubpath)
	err = os.MkdirAll(currentConfigDir, os.ModeDir)
	require.NoError(t, err)
	ext.HandlerEnv.ConfigFolder = currentConfigDir

	// --- Set up data folder structure: <root>/<extensionVersion>/downloads/<app>/<appVersion>/ ---
	dataRoot := t.TempDir()
	oldDataDir := filepath.Join(dataRoot, oldVersion, "downloads")
	appVersions := map[string]string{"appA": "1.0", "appB": "2.0"}
	for app, ver := range appVersions {
		appDir := filepath.Join(oldDataDir, app, ver)
		err = os.MkdirAll(appDir, os.ModeDir)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(appDir, "file.txt"), []byte("content-"+app), 0666)
		require.NoError(t, err)
	}
	currentDataDir := filepath.Join(dataRoot, ExtensionVersion, "downloads")
	ext.HandlerEnv.DataFolder = currentDataDir

	// --- Write a package registry file for the OLD version using the package registry ---
	// Temporarily point ConfigFolder to old config dir to write the registry there
	ext.HandlerEnv.ConfigFolder = oldConfigDir
	oldPkr, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, 1*time.Second)
	require.NoError(t, err)

	oldPackages := packageregistry.CurrentPackageRegistry{
		"appA": &packageregistry.VMAppPackageCurrent{
			ApplicationName: "appA",
			Version:         "1.0",
			DownloadDir:     filepath.ToSlash(filepath.Join(oldDataDir, "appA", "1.0")),
		},
		"appB": &packageregistry.VMAppPackageCurrent{
			ApplicationName: "appB",
			Version:         "2.0",
			DownloadDir:     filepath.ToSlash(filepath.Join(oldDataDir, "appB", "2.0")),
		},
	}
	err = oldPkr.WriteToDisk(oldPackages)
	require.NoError(t, err)
	err = oldPkr.Close()
	require.NoError(t, err)

	// Restore ConfigFolder to the current version
	ext.HandlerEnv.ConfigFolder = currentConfigDir

	// --- Call vmAppUpdateCallback ---
	err = vmAppUpdateCallback(ext)
	require.NoError(t, err)

	// --- Validation 1: Package registry file was copied to current version's config folder ---
	newRegistryPath := filepath.Join(currentConfigDir, packageregistry.LocalApplicationRegistryFileName)
	_, err = os.Stat(newRegistryPath)
	require.NoError(t, err, "package registry file should exist in the current version config folder")

	// Old version's registry should be emptied
	oldRegistryAfterUpdate, err := os.ReadFile(filepath.Join(oldConfigDir, packageregistry.LocalApplicationRegistryFileName))
	require.NoError(t, err)
	require.True(t, bytes.Equal([]byte("[]"), oldRegistryAfterUpdate),
		"old version's package registry should be overwritten with empty list")

	// --- Validation 2: Download directories were copied to current version's data folder ---
	for app, ver := range appVersions {
		copiedFile := filepath.Join(currentDataDir, app, ver, "file.txt")
		_, err = os.Stat(copiedFile)
		require.NoError(t, err, "download directory for %s should be copied to current version", app)
	}

	// --- Validation 3: DownloadDir in the registry was updated to point to current version ---
	pkr, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, 1*time.Second)
	require.NoError(t, err)
	defer pkr.Close()

	packages, err := pkr.GetExistingPackages()
	require.NoError(t, err)
	require.Len(t, packages, 2)

	for appName, pkg := range packages {
		expectedDownloadDir := filepath.Join(dataRoot, ExtensionVersion, "downloads", appName, appVersions[appName])
		require.Equal(t, expectedDownloadDir, pkg.DownloadDir,
			"DownloadDir for %s should point to current version path", appName)

		// Verify the expected files exist at the DownloadDir path
		fileContent, err := os.ReadFile(filepath.Join(pkg.DownloadDir, "file.txt"))
		require.NoError(t, err, "file.txt should exist in DownloadDir for %s", appName)
		require.Equal(t, "content-"+appName, string(fileContent),
			"file.txt content for %s should match", appName)
	}
}
