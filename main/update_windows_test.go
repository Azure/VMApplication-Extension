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
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	fileName := packageregistry.LocalApplicationRegistryFileName //gets name of application registry file
	err = createTestFiles(testFolderPath, runtimeFolderName, fileName)
	assert.NoError(t, err)
	// cleanup
	defer os.RemoveAll(testFolderPath)

	oldFileContents, err := os.ReadFile(filepath.Join(testFolderPath, "0.0.1", runtimeFolderName, fileName))
	assert.NoError(t, err)

	//call update
	err = vmAppUpdateCallback(ext)
	assert.NoError(t, err)

	oldFileContentsAfterUpdate, err := os.ReadFile(filepath.Join(testFolderPath, "0.0.1", runtimeFolderName, fileName))
	assert.NoError(t, err)

	newFileContents, err := os.ReadFile(filepath.Join(ext.HandlerEnv.ConfigFolder, fileName))
	assert.NoError(t, err)

	//checks
	assert.True(t, bytes.Equal(oldFileContents, newFileContents))
	assert.True(t, bytes.Equal([]byte("[]"), oldFileContentsAfterUpdate))
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
	assert.ErrorIs(t, err, errorExtensionVersionDirNotFound)
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
	assert.ErrorIs(t, err, errorNoOlderPakcageRegistryFileFound)
}

func Test_existingPackageRegistryFileIsNotOverwritten(t *testing.T) {
	ext := createTestVMExtension(t, []extdeserialization.VmAppSetting{})

	runtimeFolderName := "RuntimeSettings"
	testFolderPath := ext.HandlerEnv.ConfigFolder                                                                 //path to create test version folders
	ext.HandlerEnv.ConfigFolder = filepath.Join(ext.HandlerEnv.ConfigFolder, ExtensionVersion, runtimeFolderName) //overwrite to match path pattern of config folder in VM
	err := os.MkdirAll(ext.HandlerEnv.ConfigFolder, os.ModeDir)                                                   //creates new folders
	assert.NoError(t, err)
	fileName := packageregistry.LocalApplicationRegistryFileName //gets name of application registry file
	err = createTestFiles(testFolderPath, runtimeFolderName, fileName)
	assert.NoError(t, err)
	// cleanup
	defer os.RemoveAll(testFolderPath)

	fileBytes := []byte("special message")
	packageRegistryFilePath := path.Join(ext.HandlerEnv.ConfigFolder, packageregistry.LocalApplicationRegistryFileName)
	err = ioutil.WriteFile(packageRegistryFilePath, fileBytes, 0777)
	assert.NoError(t, err)
	err = vmAppUpdateCallback(ext)
	assert.NoError(t, err)
	// verify file was not overwritten
	readBytes, err := ioutil.ReadFile(packageRegistryFilePath)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(fileBytes, readBytes))
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
	err = ioutil.WriteFile(filepath.Join(folderPath, "1.0.1", runtimeFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(folderPath, "1.0.3", runtimeFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}
	testContent = []byte("Test File Contents")
	time.Sleep(time.Second)
	err = ioutil.WriteFile(filepath.Join(folderPath, "0.0.1", runtimeFolderName, fileName), testContent, 0777)
	if err != nil {
		return err
	}

	return nil
}
