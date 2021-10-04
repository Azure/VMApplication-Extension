package main

import (
	"bytes"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func Test_didFileMove(t *testing.T) {
	//set up test VM
	order := 1
	vmApplications := []VmAppSetting{
		VmAppSetting{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}
	ext := createTestVMExtension(t, vmApplications)

	//set up test files
	runtimeFolderName := "RuntimeSettings"
	testFolderPath := ext.HandlerEnv.ConfigFolder //path to create test version folders
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, extensionVersion, runtimeFolderName) //overwrite to match path pattern of config folder in VM
	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir) //creates new folders
	if err != nil {
		return
	}
	fileName := packageregistry.LocalApplicationRegistryFileName //gets name of application registry file
	err = createTestFiles(testFolderPath, runtimeFolderName, fileName)
	if err != nil {
		return
	}

	//call update
	err = vmAppUpdateCallback(ext)
	if err != nil {
		return
	}

	//checks
	assert.NoError(t, err) 	//check for errors
	isSame := compareFiles(filepath.Join(testFolderPath, "1.0.3", runtimeFolderName, fileName), filepath.Join(ext.HandlerEnv.ConfigFolder, fileName))
	assert.True(t, isSame) 	//check if correct file was moved

	//cleanup
	os.RemoveAll(maintestdir)
}

func compareFiles(path1, path2 string) bool {
	content1, err := ioutil.ReadFile(path1)
	if err != nil {
		return false
	}

	content2, err := ioutil.ReadFile(path2)
	if err != nil {
		return false
	}

	if bytes.Compare(content1, content2) == 0 {
		return true
	}

	return false
}

func createTestFiles(folderPath, runtimeFolderName, fileName string) error {
	//create test directories
	err := os.MkdirAll(filepath.Join(folderPath, "1.0.1"), os.ModeDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "0.0.1"), os.ModeDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "1.0.3", runtimeFolderName), os.ModeDir)
	if err != nil {
		return err
	}

	//creating test file
	testContent := []byte("Test File Contents")
	err = ioutil.WriteFile(filepath.Join(folderPath, "1.0.3", runtimeFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}

	return nil 
}
