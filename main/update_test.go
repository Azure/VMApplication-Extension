package main

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"testing"
)

func Test_didFileMove(t *testing.T) {
	//create test directories
	err := os.MkdirAll(folderPath + "\\" + "0.0.3", os.ModeDir)
	if err != nil {
		return
	}
	err = os.MkdirAll(folderPath+"\\"+"0.0.1", os.ModeDir)
	if err != nil {
		return
	}
	err = os.MkdirAll(folderPath+"\\"+"0.0.4"+"\\RuntimeSettings", os.ModeDir)
	if err != nil {
		return
	}

	//creating test file
	testContent := []byte("Test File")
	err = ioutil.WriteFile(folderPath+"\\"+"0.0.4"+"\\RuntimeSettings\\applicationRegistry.active", testContent, 0777)
	if err != nil {
		return
	}

	//call update
	status, err := vmAppUpdateCallback()
	if err != nil {
		return
	}

	//check if file was moved
	assert.Equal(t, "successfully created application registry file", status)

	//check if correct file was moved
	isSame := compareFiles(folderPath+"\\"+"0.0.4"+"\\RuntimeSettings\\applicationRegistry.active", folderPath+"\\"+extensionVersion+"\\RuntimeSettings\\applicationRegistry.active")
	assert.Equal(t, 0, isSame)

	//cleanup
	os.RemoveAll(folderPath)
}

func compareFiles(path1, path2 string) int {
	content1, err := ioutil.ReadFile(path1)
	if err != nil {
		return -1
	}

	content2, err := ioutil.ReadFile(path2)
	if err != nil {
		return -1
	}

	return bytes.Compare(content1, content2)
}


