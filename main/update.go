package main

import (
	"io"
	"io/ioutil"
	"os"
)

const (
	folderPath = "C:\\Packages\\Plugins\\Microsoft.CPlat.Core.VMApplicationManagerWindows"
)

func vmAppUpdateCallback() (string, error) {
	dirContent, err := ioutil.ReadDir(folderPath) //reads directory and returns in sorted order
	if err != nil {
		return "could not open directory", err
	}

	latestVersion := dirContent[len(dirContent)-1]                                                                  //taking the last version in sorted dir list
	oldFile, err := os.Open(folderPath + "\\" + latestVersion.Name() + "\\RuntimeSettings\\applicationRegistry.active") //opening the latest applicationRegistry file
	if err != nil {
		return "could not open file", err
	}
	defer oldFile.Close()

	err = os.MkdirAll(folderPath + "\\" + extensionVersion + "\\RuntimeSettings", os.ModeDir) //creating directory 
	if err != nil {
		return "could not create new file", err
	}

	newFile, err := os.Create(folderPath + "\\" + extensionVersion + "\\RuntimeSettings\\applicationRegistry.active") //creating new file
	defer newFile.Close()

	_, err = io.Copy(newFile, oldFile) //copying old registry to new
	if err != nil {
		return "could not copy file", err
	}

	return "successfully created application registry file", nil

}
