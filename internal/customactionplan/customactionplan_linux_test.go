// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package customactionplan

import (
	"fmt"
	actionplan "github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var mockCommandExecutorKillProcess CommandExecutor = func(s string, s2 string) (int, error) {
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		fmt.Print("could not find process")
	} else {
		err = proc.Signal(syscall.SIGTERM)
		if err != nil {
			fmt.Printf("could not kill process %s", err.Error())
		}
	}
	// this sleep should never be hit
	time.Sleep(5 * time.Second)
	return 0, nil
}

func executeTestInAnotherThreadAndTerminateBeforeCompletion(t *testing.T, testName, packageDir, transcriptFile string) {
	initializeTest(t)
	file, err := os.Create(transcriptFile)
	assert.NoError(t, err, "should be able to create transcript file")
	if err == nil {
		defer file.Close()
	}
	c := exec.Command("go", "test", "-v", packageDir, "-run", testName)
	c.Dir = packageDir
	c.Env = append(os.Environ(), fmt.Sprintf("%s=true", LaunchedFromAnotherProcessEnvVariable))
	c.Stdout = file
	c.Run()
}

func TestCommandExecutorCanHandleProcessBeingKilled(t *testing.T) {
	envVariables := os.Environ()
	var wasStartedByAnotherProcess = false
	for _, variable := range envVariables {
		if strings.Contains(variable, LaunchedFromAnotherProcessEnvVariable) {
			wasStartedByAnotherProcess = true
		}
	}

	action := []*extdeserialization.VmAppSetting{
		{
			ApplicationName: "app1",
			Order:           &one,
			Actions: []*extdeserialization.ActionSetting{
				{
					ActionName:   "action1",
					ActionScript: "echo hello",
					Timestamp:    "20210604T155300Z",
					Parameters:   []extdeserialization.ActionParameter{},
					TickCount:    10193113,
				},
			},
		},
	}
	newApp := packageregistry.VMAppPackageCurrent{
		ApplicationName: "app1",
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	newRegistry := packageregistry.CurrentPackageRegistry{
		"app1": &newApp,
	}

	if wasStartedByAnotherProcess {
		initializeTest(t)
		packageReg, err := packageregistry.New(extLogger, environment, time.Second)
		assert.NoError(t, err)
		if err == nil {
			defer packageReg.Close()
		}
		err = packageReg.WriteToDisk(newRegistry)
		assert.NoError(t, err)
		appPackage, err := packageReg.GetExistingPackages()

		cmdHandler := NewCommandHandlerMock(mockCommandExecutorKillProcess)
		_, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)
		packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
		assert.True(t, ok)
		assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
		assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action1", AppVersion: newApp.Version, PackageName: newApp.ApplicationName})
	} else {
		defer cleanupTest()
		currentDirAbsolutePath, err := filepath.Abs("")
		assert.NoError(t, err, "should be able to get absolute path")
		transcriptFile := path.Join(currentDirAbsolutePath, testdir, "transcript.txt")
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled", currentDirAbsolutePath, transcriptFile)
		fileContent, err := ioutil.ReadFile(transcriptFile)
		assert.Contains(t, string(fileContent), "system reboot detected")
	}
}
