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
		_, ok := statusMessage.(*PackageOperationResults)
		assert.True(t, ok)
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
		assert.Equal(t, packageregistry.NoAction, app.OngoingOperation, "OngoingOperation should be set to NoAction")
		assert.Equal(t, 0, app.NumRebootsOccurred, "Number of reboots should remain 0")
		assert.Contains(t, app.Result, "Reboot detected during 'Install' operation")
	}
}

func TestCommandExecutorCanHandleProcessBeingKilled_RerunRebootBehavior(t *testing.T) {
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
		RebootBehavior:  packageregistry.Rerun,
	}
	if wasStartedByAnotherProcess {
		initializeTest(t)

		applicationRegistryFilePath := path.Join(environment.ConfigFolder, "applicationRegistry.active")
		applicationRegistryBytes, err := ioutil.ReadFile(applicationRegistryFilePath)
		assert.NoError(t, err, "should be able to read application registry file")
		existingApps := packageregistry.VMAppPackageCurrentCollection{}

		err = json.Unmarshal(applicationRegistryBytes, &existingApps)
		assert.NoError(t, err, "should be able to deserialize existing packages")
		incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
		cmdHandler := NewCommandHandlerMock(mockCommandExecutorKillProcess)
		newReg, _, _, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
		assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
		assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
		assert.NoError(t, executeError.GetErrorIfDeploymentFailed())

	} else {
		defer cleanupTest()
		currentDirAbsolutePath, err := filepath.Abs("")
		assert.NoError(t, err, "should be able to get absolute path")
		transcriptFile := path.Join(currentDirAbsolutePath, testdir, "transcript.txt")

		// Run first time, reboot count should be incremented to 1
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled_RerunRebootBehavior", currentDirAbsolutePath, transcriptFile)
		validateApplicationAfterReboot(t, newApp.ApplicationName, 1, false)

		// Run second time, reboot count should be incremented to 2
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled_RerunRebootBehavior", currentDirAbsolutePath, transcriptFile)
		validateApplicationAfterReboot(t, newApp.ApplicationName, 2, false)

		// Run third time, reboot count should be incremented to 3
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled_RerunRebootBehavior", currentDirAbsolutePath, transcriptFile)
		validateApplicationAfterReboot(t, newApp.ApplicationName, 3, false)

		// Run fourth time. Max reboots exceeded, should fail the app. Reboot count should be reset to 0
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled_RerunRebootBehavior", currentDirAbsolutePath, transcriptFile)
		validateApplicationAfterReboot(t, newApp.ApplicationName, 0, true)
	}
}

func validateApplicationAfterReboot(t *testing.T, applicationName string, numRebootsOccurred int, failedApp bool) {
	//open the config file
	applicationRegistryFilePath := path.Join(environment.ConfigFolder, "applicationRegistry.active")
	applicationRegistryBytes, err := ioutil.ReadFile(applicationRegistryFilePath)
	assert.NoError(t, err, "should be able to read application registry file")
	existingPackages := packageregistry.VMAppPackageCurrentCollection{}

	err = json.Unmarshal(applicationRegistryBytes, &existingPackages)
	assert.NoError(t, err, "should be able to deserialize existing packages")
	app := existingPackages[0]
	assert.Equal(t, numRebootsOccurred, app.NumRebootsOccurred, "number of reboots not as intended")

	if failedApp {
		assert.Equal(t, packageregistry.Failed, app.OngoingOperation, "operation should have failed due to max reboots exceeded")
		assert.Contains(t, app.Result, "has resulted in 3 reboots. Cannot complete command.")
	} else {
		assert.Equal(t, packageregistry.Install, app.OngoingOperation, "operation should have been preserved during reboot")
		assert.Contains(t, app.Result, "Reboot detected during 'Install' operation")
	}
}
