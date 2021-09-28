package main

import (
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

func vmAppUpdateCallback(ext *vmextensionhelper.VMExtension) error {
	configFolderPath := ext.HandlerEnv.ConfigFolder
	folderPath := filepath.Dir(filepath.Dir(configFolderPath)) //directory with all versions
	
	dirContent, err := ioutil.ReadDir(folderPath) //reads directory and returns content in sorted order
	if err != nil {
		return err
	}

	oldVersion := dirContent[len(dirContent)-2]                                                                  //the version under the latest one
	oldFile, err := os.Open(filepath.Join(folderPath, oldVersion.Name(), "RuntimeSettings", "applicationRegistry.active")) //opening previous applicationRegistry file
	if err != nil {
		return err
	}
	defer oldFile.Close()

	newFile, err := os.Create(filepath.Join(configFolderPath, "applicationRegistry.active")) //creating new file
	if err != nil {
		return err
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, oldFile) //copying old registry to new
	if err != nil {
		return err
	}

	return nil
}
