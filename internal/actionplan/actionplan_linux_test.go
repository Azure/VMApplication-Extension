package actionplan

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/stretchr/testify/assert"
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
	//c := exec.Command("bash", "-c", fmt.Sprintf("go test -v %s -run %s", packageDir, testName))
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
	newApp := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           &one,
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	if wasStartedByAnotherProcess {
		initializeTest(t)

		existingApps := packageregistry.VMAppPackageCurrentCollection{}
		incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
		cmdHandler := NewCommandHandlerMock(mockCommandExecutorKillProcess)
		newReg, _, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
		assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
		assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
		assertAllActionsSucceeded(t, newReg)
		packageOperationResults, ok := statusMessage.(*PackageOperationResults)
		assert.True(t, ok)
		assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newApp.Version, PackageName: newApp.ApplicationName})
		assert.NoError(t, executeError.GetErrorIfDeploymentFailed())

	} else {
		defer cleanupTest()
		currentDirAbsolutePath, err := filepath.Abs("")
		assert.NoError(t, err, "should be able to get absolute path")
		transcriptFile := path.Join(currentDirAbsolutePath, testdir, "transcript.txt")
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled", currentDirAbsolutePath, transcriptFile)
		//open the config file
		applicationRegistryFilePath := path.Join(environment.ConfigFolder, "applicationRegistry.active")
		applicationRegistryBytes, err := ioutil.ReadFile(applicationRegistryFilePath)
		assert.NoError(t, err, "should be able to read application registry file")
		existingPackages := packageregistry.VMAppPackageCurrentCollection{}

		err = json.Unmarshal(applicationRegistryBytes, &existingPackages)
		assert.NoError(t, err, "should be able to deserialize existing packages")
		app := existingPackages[0]
		assert.Equal(t, packageregistry.Install, app.OngoingOperation, "OngoingOperation should be preserved if reboot is detected")
		assert.Equal(t, 1, app.NumRebootsOccurred, "Number of reboots should have been incremented")
		assert.Contains(t, app.Result, "Reboot detected during 'Install' operation")
	}
}

func TestCommandExecutorCanHandleProcessBeingKilled_MaxRebootsExceeded(t *testing.T) {
	envVariables := os.Environ()
	var wasStartedByAnotherProcess = false
	for _, variable := range envVariables {
		if strings.Contains(variable, LaunchedFromAnotherProcessEnvVariable) {
			wasStartedByAnotherProcess = true
		}
	}
	oldApp := packageregistry.VMAppPackageCurrent{
		ApplicationName:    "app1",
		Version:            "1.0",
		InstallCommand:     "install app1",
		RemoveCommand:      "remove app1",
		UpdateCommand:      "update app1",
		OngoingOperation:   packageregistry.Install,
		NumRebootsOccurred: 3,
	}
	newApp := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           &one,
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	if wasStartedByAnotherProcess {
		initializeTest(t)

		existingApps := packageregistry.VMAppPackageCurrentCollection{&oldApp}
		incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
		cmdHandler := NewCommandHandlerMock(mockCommandExecutorKillProcess)
		newReg, _, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
		assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
		assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
		_, ok := statusMessage.(*PackageOperationResults)
		assert.True(t, ok)
		assert.NoError(t, executeError.GetErrorIfDeploymentFailed())

	} else {
		defer cleanupTest()
		currentDirAbsolutePath, err := filepath.Abs("")
		assert.NoError(t, err, "should be able to get absolute path")
		transcriptFile := path.Join(currentDirAbsolutePath, testdir, "transcript.txt")
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled_MaxRebootsExceeded", currentDirAbsolutePath, transcriptFile)
		//open the config file
		applicationRegistryFilePath := path.Join(environment.ConfigFolder, "applicationRegistry.active")
		applicationRegistryBytes, err := ioutil.ReadFile(applicationRegistryFilePath)
		assert.NoError(t, err, "should be able to read application registry file")
		existingPackages := packageregistry.VMAppPackageCurrentCollection{}

		err = json.Unmarshal(applicationRegistryBytes, &existingPackages)
		assert.NoError(t, err, "should be able to deserialize existing packages")
		app := existingPackages[0]
		assert.Equal(t, packageregistry.Failed, app.OngoingOperation, "OngoingOperation should be set to failed")
		assert.Equal(t, 0, app.NumRebootsOccurred, "Number of reboots should have been reset")
		assert.Contains(t, app.Result, "has resulted in 3 reboots. Cannot complete command.")
	}
}
