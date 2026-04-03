// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/Azure/VMApplication-Extension/pkg/utils"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

var (
	errorExtensionVersionDirNotFound     = errors.New("could not find the directory that contains all the extension versions")
	errorNoOlderPakcageRegistryFileFound = errors.New(fmt.Sprintf("could not find an older '%s' file", packageregistry.LocalApplicationRegistryFileName))
	emptyPackageRegistryContent          = []byte("[]")
)

type FileInfoWithFilePath struct {
	fileInfo os.FileInfo
	filePath string
}

type SortableFileInfoImpl struct {
	FileInfoArray []FileInfoWithFilePath
}
type SortableFileInfo interface {
	Len() int
	Less(i, j int) bool
	Swap(i, j int)
}

func (sortableFileInfo SortableFileInfoImpl) Len() int {
	return len(sortableFileInfo.FileInfoArray)
}

func (sortableFileInfo SortableFileInfoImpl) Less(i, j int) bool {
	return sortableFileInfo.FileInfoArray[i].fileInfo.ModTime().Before(sortableFileInfo.FileInfoArray[j].fileInfo.ModTime())
}

func (sortableFileInfo SortableFileInfoImpl) Swap(i, j int) {
	swapVar := sortableFileInfo.FileInfoArray[i]
	sortableFileInfo.FileInfoArray[i] = sortableFileInfo.FileInfoArray[j]
	sortableFileInfo.FileInfoArray[j] = swapVar
}

func getMostRecentlyUpdatedPackageRegistryFile(dirContainingAllVersions string, intermediatePath string) (string, error) {
	fileInfo, err := os.ReadDir(dirContainingAllVersions) //reads directory and returns content in sorted order
	if err != nil {
		return "", err
	}
	sortableRegistryFileInfo := SortableFileInfoImpl{
		FileInfoArray: []FileInfoWithFilePath{},
	}
	for _, fileInfo := range fileInfo {
		if fileInfo.IsDir() && fileInfo.Name() != ExtensionVersion && utils.IsValidVersionString(fileInfo.Name()) {
			registryFilePath := filepath.Join(dirContainingAllVersions, fileInfo.Name(), intermediatePath, packageregistry.LocalApplicationRegistryFileName)
			registryFileInfo, err := os.Stat(registryFilePath)
			if err == nil {
				sortableRegistryFileInfo.FileInfoArray = append(sortableRegistryFileInfo.FileInfoArray, FileInfoWithFilePath{registryFileInfo, registryFilePath})
			}
		}
	}
	if sortableRegistryFileInfo.Len() < 1 {
		return "", errorNoOlderPakcageRegistryFileFound
	}
	sort.Sort(sortableRegistryFileInfo)
	return sortableRegistryFileInfo.FileInfoArray[len(sortableRegistryFileInfo.FileInfoArray)-1].filePath, nil
}

// findVersionDir walks up from configFolder to find a directory whose name is a version string.
// Returns the parent directory (containing all versions) and the relative path from the version dir down to configFolder.
func findVersionDir(configFolder string) (string, string, error) {
	relativePathToConfigFolder := ""
	for currentFolderPath := configFolder; currentFolderPath != filepath.Dir(currentFolderPath); currentFolderPath = filepath.Dir(currentFolderPath) {
		currentFolderName := filepath.Base(currentFolderPath)
		if utils.IsValidVersionString(currentFolderName) {
			return filepath.Dir(currentFolderPath), relativePathToConfigFolder, nil
		}
		relativePathToConfigFolder = filepath.Join(currentFolderName, relativePathToConfigFolder)
	}
	return "", "", errorExtensionVersionDirNotFound
}

func vmAppUpdateCallback(ext *vmextensionhelper.VMExtension) error {
	// for extension update on windows, we retrieve the applicationRegistry.active file from a previous version of the extension

	packageRegistryFilePathForCurrentVersion := filepath.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	_, err := os.Stat(packageRegistryFilePathForCurrentVersion)
	if !os.IsNotExist(err) {
		// a package registry file already exists for current version, nothing to do
		return nil
	}

	folderPathThatContainsAllTheVersions, relativePathToConfigFolder, err := findVersionDir(ext.HandlerEnv.ConfigFolder)
	if err != nil {
		return err
	}

	previousPackageRegistryFilePath, err := getMostRecentlyUpdatedPackageRegistryFile(folderPathThatContainsAllTheVersions, relativePathToConfigFolder)
	if err != nil {
		return err
	}

	previousPackageRegistryContent, err := os.ReadFile(previousPackageRegistryFilePath)
	if err != nil {
		return err
	}

	// Creates and writes previous registry content to package registry file for new extension version
	err = os.WriteFile(packageRegistryFilePathForCurrentVersion, previousPackageRegistryContent, 0666)
	if err != nil {
		return err
	}

	// Overwrite the package registry for older version to be an empty list of applications
	err = os.WriteFile(previousPackageRegistryFilePath, emptyPackageRegistryContent, 0666)

	// do the following operations in a best effort manner
	err = moveDownloadDirToCurrentVersion(ext)

	if err != nil {
		ext.ExtensionLogger.Warn("Failed to move download directory to current version with error: %v", err)
		ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to move download directory to current version with error: %v", err))
	} else if err = updateDonwnloadDirInPackageRegistryFile(ext); err != nil {
		ext.ExtensionLogger.Warn("Failed to update download directory in package registry file with error: %v", err)
		ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to update download directory in package registry file with error: %v", err))
	}
	return nil
}

func updateDonwnloadDirInPackageRegistryFile(ext *vmextensionhelper.VMExtension) error {
	packageRegistry, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		return err
	}
	defer packageRegistry.Close()
	existingPackages, err := packageRegistry.GetExistingPackages()
	if err != nil {
		return err
	}

	if len(existingPackages) == 0 {
		return nil
	}

	downloadDirBeforeVersion, downloadDirAfterVersion, err := findVersionDir(ext.HandlerEnv.DataFolder)
	if err != nil {
		return err
	}

	// Build a regex that matches: <prefix>/<any_version>/<suffix> in DownloadDir paths
	// Use forward slashes since DownloadDir is stored with filepath.ToSlash
	escapedPrefix := regexp.QuoteMeta(filepath.ToSlash(downloadDirBeforeVersion))
	escapedSuffix := regexp.QuoteMeta(filepath.ToSlash(downloadDirAfterVersion))
	downloadDirVersionRegex := regexp.MustCompile(escapedPrefix + `/[^/]+/` + escapedSuffix)
	replacement := filepath.ToSlash(downloadDirBeforeVersion) + "/" + ExtensionVersion + "/" + filepath.ToSlash(downloadDirAfterVersion)

	for packageName, packageInfo := range existingPackages {
		normalized := filepath.ToSlash(packageInfo.DownloadDir)
		updated := downloadDirVersionRegex.ReplaceAllLiteralString(normalized, replacement)
		if updated == normalized {
			ext.ExtensionLogger.Warn("Could not update downloadDir for package '%s', no version segment matched", packageName)
			ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Could not update downloadDir for package '%s', no version segment matched", packageName))
		} else {
			packageInfo.DownloadDir = filepath.FromSlash(updated)
		}
	}

	return packageRegistry.WriteToDisk(existingPackages)
}

// move the download directory from old version to new version
func moveDownloadDirToCurrentVersion(ext *vmextensionhelper.VMExtension) error {
	packageRegistry, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		return err
	}
	defer packageRegistry.Close()

	rootOfAllVersions, relativePathAfterVersion, err := findVersionDir(ext.HandlerEnv.DataFolder)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(rootOfAllVersions)
	if err != nil {
		return err
	}
	var downloadDirectoryForAllVersions []string
	for _, entry := range entries {
		if entry.IsDir() && utils.IsValidVersionString(entry.Name()) {
			dirName := filepath.Join(rootOfAllVersions, entry.Name(), relativePathAfterVersion)
			downloadDir, err := os.Stat(dirName)
			if err != nil {
				ext.ExtensionLogger.Warn("Skipping directory %s when looking for download directories to move, with error: %v", dirName, err)
				ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Skipping directory %s when looking for download directories to move, with error: %v", dirName, err))
				continue
			}
			if !downloadDir.IsDir() || entry.Name() == ExtensionVersion {
				//if the config folder is not a directory, or if the version folder is the same as the current version, then skip it
				continue
			}
			downloadDirectoryForAllVersions = append(downloadDirectoryForAllVersions, dirName)
		}
	}

	for _, downloadDir := range downloadDirectoryForAllVersions {
		directoryContents, err := os.ReadDir(downloadDir)
		if err != nil {
			ext.ExtensionLogger.Warn("Failed to read directory %s with error: %v", downloadDir, err)
			ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to read directory %s with error: %v", downloadDir, err))
			continue
		}
		for _, entry := range directoryContents {
			if entry.IsDir() {
				sourceDirFullPath := filepath.Join(downloadDir, entry.Name())
				destDirFullPath := filepath.Join(ext.HandlerEnv.DataFolder, entry.Name())
				err = copySubdirectoryUsingRobocopy(sourceDirFullPath, destDirFullPath)
				// copy the directory from current entry to ext.HandlerEnv.DataFolder
				if err != nil {
					ext.ExtensionLogger.Warn("Failed to copy directory from %s to %s with error: %s", sourceDirFullPath, destDirFullPath, err.Error())
					ext.ExtensionEvents.LogWarningEvent("vm-application-manager-update", fmt.Sprintf("Failed to copy directory from %s to %s with error: %s", sourceDirFullPath, destDirFullPath, err.Error()))
				}
			}
		}
	}

	return nil
}

func copySubdirectoryUsingRobocopy(src, dst string) error {
	cmd := exec.Command("robocopy", src, dst, "/E", "/sl", "/NFL", "/NDL", "/NJH", "/NJS")
	err := cmd.Run()
	// robocopy exit codes 0-7 are success/informational
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() < 8 {
		return nil
	}
	return err
}
