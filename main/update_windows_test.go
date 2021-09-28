package main

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func Test_didFileMove(t *testing.T) {
	//set up test VM
	maintestdir = filepath.Join("C:\\Microsoft.CPlat.Core.VMApplicationManagerWindows", extensionVersion)
	order := 1
	vmApplications := []VmAppSetting{
		VmAppSetting{
			ApplicationName: "iggy",
			Order:           &order,
		},
	}
	ext := createTestVMExtension(t, vmApplications)

	//get folder paths and set up test files
	configFolderPath := ext.HandlerEnv.ConfigFolder
	folderPath := filepath.Dir(filepath.Dir(configFolderPath))
	err := createTestFiles(folderPath)
	if err != nil {
		return
	}

	//call update
	err = vmAppUpdateCallback(ext)
	if err != nil {
		return
	}

	//check if file was moved
	assert.NoError(t, err)
	//check if correct file was moved
	isSame := compareFiles(filepath.Join(folderPath,"1.0.3","RuntimeSettings","applicationRegistry.active"), filepath.Join(configFolderPath, "applicationRegistry.active"))
	assert.True(t, isSame)

	//cleanup
	os.RemoveAll(folderPath)
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

func createTestFiles(folderPath string) error {
	//create test directories
	err := os.MkdirAll(filepath.Join(folderPath, "1.0.1"), os.ModeDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "0.0.1"), os.ModeDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "1.0.3", "RuntimeSettings" ), os.ModeDir)
	if err != nil {
		return err
	}

	//creating test file
	testContent := []byte("Test File Contents")
	err = ioutil.WriteFile(filepath.Join(folderPath,"1.0.3","RuntimeSettings","applicationRegistry.active"), testContent, 0777)
	if err != nil {
		return err
	}

	return nil 
}
