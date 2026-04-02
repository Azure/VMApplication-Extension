// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
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
			registryFilePath := path.Join(dirContainingAllVersions, fileInfo.Name(), intermediatePath, packageregistry.LocalApplicationRegistryFileName)
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

func vmAppUpdateCallback(ext *vmextensionhelper.VMExtension) error {
	// for extension update on windows, we retrieve the applicationRegistry.active file from a previous version of the extension

	packageRegistryFilePathForCurrentVersion := filepath.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	_, err := os.Stat(packageRegistryFilePathForCurrentVersion)
	if !os.IsNotExist(err) {
		// a package registry file already exists for current version, nothing to do
		return nil
	}

	// Walk up the directory tree to find the directory whose name contains ExtensionVersion,
	// tracking the relative path below it in pathToFile.
	folderPathThatContainsAllTheVersions := ""
	relativePathToConfigFolder := ""
	for currentFolderPath := ext.HandlerEnv.ConfigFolder; currentFolderPath != filepath.Dir(currentFolderPath); currentFolderPath = filepath.Dir(currentFolderPath) {
		currentFolderName := filepath.Base(currentFolderPath)

		if utils.IsValidVersionString(currentFolderName) {
			folderPathThatContainsAllTheVersions = filepath.Dir(currentFolderPath) // if the leaf folder is a version number, then the parent folder should be the one that contains all the versions
			break
		}
		relativePathToConfigFolder = path.Join(currentFolderName, relativePathToConfigFolder) // build it from leaf to root since we are walking up the directory tree
	}
	if folderPathThatContainsAllTheVersions == "" {
		return errorExtensionVersionDirNotFound
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

	// TODO: Copy the download dir from the older version into a path more appropriate for the new version,
	// but this is not urgent since the download dir is only a cache and will be repopulated as needed.

	// TODO: Update the references in package registry file to point to the new download dir if we do copy the download dir,
	// but again this is not urgent since the download dir will be repopulated as needed.

	return nil
}
