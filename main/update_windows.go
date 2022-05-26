package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

var (
	errorExtensionVersionDirNotFound     = errors.New("could not find the directory that contains all the extension versions")
	errorNoOlderPakcageRegistryFileFound = errors.New(fmt.Sprintf("could not find an older '%s' file", packageregistry.LocalApplicationRegistryFileName))
	versionNumberRegx, _                 = regexp.Compile(`[0-9]+\.[0-9]+\.[0-9]+`)
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
	fileInfo, err := ioutil.ReadDir(dirContainingAllVersions) //reads directory and returns content in sorted order
	if err != nil {
		return "", err
	}
	sortableRegistryFileInfo := SortableFileInfoImpl{
		FileInfoArray: []FileInfoWithFilePath{},
	}
	for _, fileInfo := range fileInfo {
		if fileInfo.IsDir() && versionNumberRegx.MatchString(fileInfo.Name()) {
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
	folderPath := ext.HandlerEnv.ConfigFolder
	currentFolderName := ""
	pathToFile := ""

	//loop to find directory that contains current version
	breakLoopAfter := 5
	for i := 0; ; i++ {
		currentFolderName = filepath.Base(folderPath)
		if strings.Contains(currentFolderName, extensionVersion) {
			break
		}
		pathToFile = filepath.Join(currentFolderName, pathToFile) //keeping track of full path to file
		folderPath = filepath.Dir(folderPath)                     //update folderpath to walk up directory
		if i > breakLoopAfter {
			return errorExtensionVersionDirNotFound
		}
	}

	folderPath = filepath.Dir(folderPath) //folder that contains all the versions

	previousPackageFilePath, err := getMostRecentlyUpdatedPackageRegistryFile(folderPath, pathToFile)
	if err != nil {
		return err
	}

	prevFile, err := os.Open(previousPackageFilePath) //opening the applicationRegistry file
	if err != nil {
		return err
	}
	defer prevFile.Close()

	newFile, err := os.Create(filepath.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)) //creating new file
	if err != nil {
		return err
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, prevFile) //copying previous registry to new
	if err != nil {
		return err
	}

	return nil
}
