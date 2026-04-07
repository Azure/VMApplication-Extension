package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
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

// splitPathAroundVersionedDir splits dirpath into (head, versionedDirName, tail) by walking up to find an ancestor
// directory whose name matches one of the version-checking functions.
func splitPathAroundVersionedDir(dirpath string,
	dirnameCheckers []func(currentFolderName string) bool) (
	head,
	versionedDirName,
	tail string,
	errorToReturn error) {
	// contains an array of comparison functions that will be run to determine the version dir
	// to have robustness, if the first way of comparison fails, use the next one

	for _, checkDirName := range dirnameCheckers {
		relativePathToConfigFolder := ""
		for currentFolderPath := dirpath; currentFolderPath != filepath.Dir(currentFolderPath); currentFolderPath = filepath.Dir(currentFolderPath) {
			currentFolderName := filepath.Base(currentFolderPath)
			if checkDirName(currentFolderName) {
				head = filepath.Dir(currentFolderPath)
				versionedDirName = currentFolderName
				tail = relativePathToConfigFolder
				errorToReturn = nil
				return
			}
			relativePathToConfigFolder = filepath.Join(currentFolderName, relativePathToConfigFolder)
		}
	}
	head = ""
	versionedDirName = ""
	tail = ""
	errorToReturn = errorExtensionVersionDirNotFound
	return
}

func getMostRecentlyUpdatedPackageRegistryFile(dirContainingAllVersions string, intermediatePath string, expectedDirNamePatternChecker func(string) bool) (string, error) {
	fileInfo, err := os.ReadDir(dirContainingAllVersions) //reads directory and returns content in sorted order
	if err != nil {
		return "", err
	}
	sortableRegistryFileInfo := SortableFileInfoImpl{
		FileInfoArray: []FileInfoWithFilePath{},
	}
	for _, fileInfo := range fileInfo {
		if fileInfo.IsDir() && fileInfo.Name() != ExtensionVersion && expectedDirNamePatternChecker(fileInfo.Name()) {
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
