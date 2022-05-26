package main

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

var ErrorExtensionVersionDirNotFound = errors.New("could not find the directory that contains all the extension versions")

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
			return ErrorExtensionVersionDirNotFound
		}
	}

	folderPath = filepath.Dir(folderPath)         //folder that contains all the versions
	dirContent, err := ioutil.ReadDir(folderPath) //reads directory and returns content in sorted order
	if err != nil {
		return err
	}
	if len(dirContent) < 2 { //checks if directory contains siblings (other versions)
		return errors.New("directory does not contain previous version")
	}

	fileName := packageregistry.LocalApplicationRegistryFileName
	prevVersionFolder := dirContent[len(dirContent)-2]                                                  //taking the version under latest
	prevFile, err := os.Open(filepath.Join(folderPath, prevVersionFolder.Name(), pathToFile, fileName)) //opening the applicationRegistry file
	if err != nil {
		return err
	}
	defer prevFile.Close()

	newFile, err := os.Create(filepath.Join(ext.HandlerEnv.ConfigFolder, fileName)) //creating new file
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
