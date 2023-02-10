package actionplan

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const LaunchedFromAnotherProcessEnvVariable = "LAUNCHED_FROM_ANOTHER_PROCESS"
const errorString = "command failed as expected"

var one = 1
var el = logging.New(nil)

type CommandExecutor func(string, string) (int, error)

type commandResult struct {
	command    string
	returnCode int
	err        error
}

type CommandHandlerMock struct {
	Result   []commandResult
	Executor CommandExecutor
}

func NewCommandHandlerMock(executor func(string, string) (int, error)) *CommandHandlerMock {
	return &CommandHandlerMock{Result: []commandResult{}, Executor: executor}
}

func (commandHandlerMock *CommandHandlerMock) Execute(command string, workingDir, logDir string, waitForCompletion bool, el *logging.ExtensionLogger) (returnCode int, err error) {
	returnCode, err = commandHandlerMock.Executor(command, workingDir)
	commandHandlerMock.Result = append(commandHandlerMock.Result, commandResult{command, returnCode, err})
	return
}

var mockCommandExecutorNoError CommandExecutor = func(string, string) (int, error) {
	return 0, nil
}

var mockCommandFailOnDemand CommandExecutor = func(command string, workingDir string) (int, error) {
	if strings.HasPrefix(command, "fail") {
		return -1, errors.Errorf(errorString)
	}
	return 0, nil
}

// implements IHostGaCommunicator
type NoopHostGaComminucator struct{}

func (downloader *NoopHostGaComminucator) DownloadPackage(logger *logging.ExtensionLogger, appName string, dst string) error {
	return nil
}
func (downloader *NoopHostGaComminucator) DownloadConfig(logger *logging.ExtensionLogger, appName string, dst string) error {
	return nil
}
func (downloader *NoopHostGaComminucator) GetVMAppInfo(logger *logging.ExtensionLogger, appName string) (*hostgacommunicator.VMAppMetadata, error) {
	return nil, nil
}

var environment = &handlerenv.HandlerEnvironment{
	DataFolder:   path.Join(testdir, "data"),
	ConfigFolder: path.Join(testdir, "config"),
}

func initializeTest(t *testing.T) {
	err := os.MkdirAll(environment.ConfigFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		os.Stderr.WriteString("could not create handler environment config directory")
		t.Fatal(err)
	}

	err = os.MkdirAll(environment.DataFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		os.Stderr.WriteString("could not create handler environment config directory")
		t.Fatal(err)
	}
}

func cleanupTest() {
	os.RemoveAll(testdir)
}

func getVmAppPackageCurrent(appName string, appVersion string) packageregistry.VMAppPackageCurrent {
	return packageregistry.VMAppPackageCurrent{
		ApplicationName:  appName,
		Version:          appVersion,
		InstallCommand:   fmt.Sprintf("install %v %v", appName, appVersion),
		RemoveCommand:    fmt.Sprintf("remove %v %v", appName, appVersion),
		UpdateCommand:    fmt.Sprintf("update %v %v", appName, appVersion),
		OngoingOperation: packageregistry.NoAction,
	}
}

func TestSingleInstallWithOrder(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	newApp := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           &one,
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, _, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newApp.Version, PackageName: newApp.ApplicationName})
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
}

func TestSingleInstallWithoutOrder(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	newApp := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app2",
		Order:           nil,
		Version:         "1.0",
		InstallCommand:  "install app2",
		RemoveCommand:   "remove app2",
		UpdateCommand:   "update app2",
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, _, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newApp.Version, PackageName: newApp.ApplicationName})
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
}

func TestSingleRemove(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	currentVmApp := getVmAppPackageCurrent("app1", "1.0")

	existingApps := packageregistry.VMAppPackageCurrentCollection{&currentVmApp}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, _, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, 0, len(newReg)) // the current registry should have no applications
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Remove.ToString(), AppVersion: currentVmApp.Version, PackageName: currentVmApp.ApplicationName})
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
}

func TestTreatFailureAsDeploymentFailureSingle(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	// test install
	newApp := packageregistry.VMAppPackageIncoming{
		ApplicationName:                 "app1",
		Order:                           &one,
		Version:                         "1.0",
		InstallCommand:                  "failinstall app1",
		RemoveCommand:                   "remove app1",
		UpdateCommand:                   "failupdate app1",
		TreatFailureAsDeploymentFailure: true,
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
	cmdHandler := NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, _, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.Contains(t, (*packageOperationResults)[0].Result, "Error")
	assert.Error(t, executeError.GetErrorIfDeploymentFailed())
	assert.Equal(t, newApp.ApplicationName, executeError.failedDeploymentErr.appsWithTreatFailureAsDeploymentFailure[0])
	errorMessage := executeError.GetErrorIfDeploymentFailed().Error()
	assert.Contains(t, errorMessage, newApp.ApplicationName)

	//test explicit update
	oldApp := getVmAppPackageCurrent("app1", "0.9")
	existingApps = packageregistry.VMAppPackageCurrentCollection{&oldApp}
	cmdHandler = NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, _, statusMessage, executeError = executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assert.Equal(t, newApp.UpdateCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	packageOperationResults, ok = statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.Contains(t, (*packageOperationResults)[0].Result, "Error")
	assert.Error(t, executeError.GetErrorIfDeploymentFailed())
	assert.Equal(t, newApp.ApplicationName, executeError.failedDeploymentErr.appsWithTreatFailureAsDeploymentFailure[0])

	//test implicit update
	newApp.UpdateCommand = ""
	cmdHandler = NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, _, statusMessage, executeError = executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assert.Equal(t, 2, len(cmdHandler.Result))
	assert.Equal(t, newApp.InstallCommand, cmdHandler.Result[1].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	packageOperationResults, ok = statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.Contains(t, (*packageOperationResults)[1].Result, "Error")
	assert.Error(t, executeError.GetErrorIfDeploymentFailed())
	assert.Equal(t, newApp.ApplicationName, executeError.failedDeploymentErr.appsWithTreatFailureAsDeploymentFailure[0])
}

func TestTreatFailureAsDeploymentFailureMultiple(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	// test install
	newApp1 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "failupdate app1",
	}
	newApp2 := packageregistry.VMAppPackageIncoming{
		ApplicationName:                 "app2",
		Version:                         "2.0",
		InstallCommand:                  "failinstall app2",
		RemoveCommand:                   "remove app2",
		UpdateCommand:                   "failupdate app2",
		TreatFailureAsDeploymentFailure: true,
	}
	newApp3 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app3",
		Version:         "3.0",
		InstallCommand:  "failinstall app3",
		RemoveCommand:   "remove app3",
		UpdateCommand:   "failupdate app3",
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp1, &newApp2, &newApp3}
	cmdHandler := NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, _, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, 3, len(cmdHandler.Result))
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.Equal(t, 3, len(*packageOperationResults), "Error")
	assert.Error(t, executeError.GetErrorIfDeploymentFailed())
	assert.Equal(t, 1, len(executeError.failedDeploymentErr.appsWithTreatFailureAsDeploymentFailure))
	assert.Equal(t, newApp2.ApplicationName, executeError.failedDeploymentErr.appsWithTreatFailureAsDeploymentFailure[0])
}

func TestUpdateCommandIsCalledWhenPresent(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	oldVersion := getVmAppPackageCurrent("app1", "1.0")

	newVersion := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           nil,
		Version:         "1.1",
		InstallCommand:  "install app1 1.1",
		RemoveCommand:   "remove app1 1.1",
		UpdateCommand:   "update app1 1.1",
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{&oldVersion}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newVersion}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 1, len(actionPlan.unorderedOperations[0]), "there must be 1 dependent actions")
	assert.Equal(t, 1, len(cmdHandler.Result), "1 command must be invoked")
	assert.Equal(t, newVersion.UpdateCommand, cmdHandler.Result[0].command, "the update method for the new app version must be called")

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Update.ToString(), AppVersion: newVersion.Version, PackageName: newVersion.ApplicationName})

	// test the same for ordered actions
	newVersion.Order = &one
	cmdHandler = NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan, statusMessage, executeError = executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.orderedOperations), "there must be 1 ordered operation")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[one]), "there must be only one set of dependent actions for order == 1")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[one][0]), "there must be 1 dependent action")
	assert.Equal(t, 1, len(cmdHandler.Result), "1 command must be invoked")
	assert.Equal(t, newVersion.UpdateCommand, cmdHandler.Result[0].command, "the update method for the new app version must be called")

	packageOperationResults, ok = statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Update.ToString(), AppVersion: newVersion.Version, PackageName: newVersion.ApplicationName})
}

func TestDependentActionsAreCreatedForUpdatesWithoutUpdateCommand(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	oldVersion := getVmAppPackageCurrent("app1", "1.0")

	newVersion := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           nil,
		Version:         "1.1",
		InstallCommand:  "install app1 1.1",
		RemoveCommand:   "remove app1 1.1",
		UpdateCommand:   "",
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{&oldVersion}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newVersion}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 2, len(actionPlan.unorderedOperations[0]), "there must be 2 dependent actions")
	assert.Equal(t, 2, len(cmdHandler.Result), "2 commands must be invoked")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, newVersion.InstallCommand, cmdHandler.Result[1].command, "the install method for the new app version must be called")

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.RemoveForUpdate.ToString(), AppVersion: oldVersion.Version, PackageName: oldVersion.ApplicationName})
	assert.EqualValues(t, (*packageOperationResults)[1], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newVersion.Version, PackageName: newVersion.ApplicationName})

	// test the same for ordered actions
	newVersion.Order = &one
	cmdHandler = NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan, statusMessage, executeError = executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.orderedOperations), "there must be only one ordered operation")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[one]), "there must be only one set of dependent actions for order == 1")
	assert.Equal(t, 2, len(actionPlan.orderedOperations[one][0]), "there must be 2 dependent actions")
	assert.Equal(t, 2, len(cmdHandler.Result), "2 commands must be invoked")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, newVersion.InstallCommand, cmdHandler.Result[1].command, "the install method for the new app version must be called")

	packageOperationResults, ok = statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.RemoveForUpdate.ToString(), AppVersion: oldVersion.Version, PackageName: oldVersion.ApplicationName})
	assert.EqualValues(t, (*packageOperationResults)[1], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newVersion.Version, PackageName: newVersion.ApplicationName})
}

func TestDependantActionsAreCancelled(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	oldVersion := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "1.0",
		InstallCommand:   "install app1 1.0",
		RemoveCommand:    "failremove app1 1.0",
		UpdateCommand:    "update app1 1.0",
		OngoingOperation: packageregistry.NoAction,
	}

	newVersion := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           nil,
		Version:         "1.1",
		InstallCommand:  "install app1 1.1",
		RemoveCommand:   "remove app1 1.1",
		UpdateCommand:   "",
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{&oldVersion}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newVersion}
	cmdHandler := NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, actionPlan, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 2, len(actionPlan.unorderedOperations[0]), "there must be 2 dependent actions")
	assert.Equal(t, 1, len(cmdHandler.Result), "only one command should have been executed")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, packageregistry.Failed, newReg[oldVersion.ApplicationName].OngoingOperation, "the package status should be failed")
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.Contains(t, (*packageOperationResults)[0].Result, errorString)
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
}

func TestRemovedAppsAreRemovedFromRegistryEvenWhenFailed(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	old1 := getVmAppPackageCurrent("app1", "1.0")
	old2 := getVmAppPackageCurrent("app2", "1.0")
	old3 := getVmAppPackageCurrent("app3", "1.0")
	old2.RemoveCommand = "fail"

	// Existing state: 3 apps installed. Desired state: no apps installed
	existingApps := packageregistry.VMAppPackageCurrentCollection{&old1, &old2, &old3}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{}
	cmdHandler := NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, actionPlan, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 3, len(actionPlan.unorderedImplicitUninstalls), "we are expecting 3 unordered operations")
	assert.Equal(t, 3, len(cmdHandler.Result), "3 total command executions are expected")
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())

	expectedError := "Error executing command 'fail': command failed as expected"
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	verifyPackageOperationResult(t, &PackageOperationResult{Result: Success, Operation: packageregistry.Remove.ToString(), AppVersion: old1.Version, PackageName: old1.ApplicationName}, packageOperationResults)
	verifyPackageOperationResult(t, &PackageOperationResult{Result: expectedError, Operation: packageregistry.Remove.ToString(), AppVersion: old2.Version, PackageName: old2.ApplicationName}, packageOperationResults)
	verifyPackageOperationResult(t, &PackageOperationResult{Result: Success, Operation: packageregistry.Remove.ToString(), AppVersion: old3.Version, PackageName: old3.ApplicationName}, packageOperationResults)
}

func verifyPackageOperationResult(t *testing.T, expectedResult *PackageOperationResult, actualResults *PackageOperationResults) {
	for i := 0; i < len((*actualResults)); i++ {
		if expectedResult.PackageName == (*actualResults)[i].PackageName && expectedResult.AppVersion == (*actualResults)[i].AppVersion {
			assert.EqualValues(t, (*expectedResult), (*actualResults)[i])
		}
	}
}

func TestOrderIsMaintainedAndHigherOrderOperationsAreSkippedOnFailure(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	old1 := getVmAppPackageCurrent("app1", "1.0")
	old2 := getVmAppPackageCurrent("app2", "1.0")
	old3 := getVmAppPackageCurrent("app3", "1.0")

	new1 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Version:         "1.1",
		InstallCommand:  "install app1 1.0",
		RemoveCommand:   "remove app1 1.0",
		UpdateCommand:   "update app1 1.0",
		Order:           &one,
	}
	two := 2
	three := 3
	new2 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app2",
		Version:         "1.5.6",
		InstallCommand:  "install app2 1.5.6",
		RemoveCommand:   "remove app2 1.5.6",
		UpdateCommand:   "update app2 1.5.6",
		Order:           &three,
	}

	new4 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app4",
		Version:         "1.0",
		InstallCommand:  "install app4 1.0",
		RemoveCommand:   "remove app4 1.0",
		UpdateCommand:   "update app4 1.0",
		Order:           &two,
	}
	new5 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app5",
		Version:         "1.0",
		InstallCommand:  "install app5 1.0",
		RemoveCommand:   "remove app5 1.0",
		UpdateCommand:   "update app5 1.0",
		Order:           &two,
	}

	existingApps := packageregistry.VMAppPackageCurrentCollection{&old1, &old2, &old3}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&new1, &new2, &new4, &new5}
	cmdHandler := NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, actionPlan, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assertAllActionsSucceeded(t, newReg)
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.unorderedImplicitUninstalls), "we are expecting one uninstall")
	assert.Equal(t, 3, len(actionPlan.orderedOperations), "we are expecting 3 ordered operations")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[1]), "1 operation of order 1")
	assert.Equal(t, 2, len(actionPlan.orderedOperations[2]), "2 operations of order 2")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[3]), "1 operation of order 3")
	assert.Equal(t, 5, len(cmdHandler.Result), "5 total command executions are expected")
	assert.Equal(t, old3.RemoveCommand, cmdHandler.Result[0].command, "first command should be old3 remove")
	assert.Equal(t, new1.UpdateCommand, cmdHandler.Result[1].command, "second command should be new1 update")
	assert.True(t, cmdHandler.Result[2].command == new4.InstallCommand || cmdHandler.Result[2].command == new5.InstallCommand,
		"third command should be new4 install or new 5 install")
	assert.True(t, cmdHandler.Result[3].command == new4.InstallCommand || cmdHandler.Result[3].command == new5.InstallCommand,
		"fourth command should be new4 install or new 5 install")
	assert.Equal(t, new2.UpdateCommand, cmdHandler.Result[4].command, "fifth command should be new 2 update")

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Remove.ToString(), AppVersion: old3.Version, PackageName: old3.ApplicationName})
	assert.EqualValues(t, (*packageOperationResults)[1], PackageOperationResult{Result: Success, Operation: packageregistry.Update.ToString(), AppVersion: new1.Version, PackageName: new1.ApplicationName})
	assert.EqualValues(t, (*packageOperationResults)[4], PackageOperationResult{Result: Success, Operation: packageregistry.Update.ToString(), AppVersion: new2.Version, PackageName: new2.ApplicationName})

	// test that failure skips higher order
	newFail6 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app6",
		Version:         "1.0",
		InstallCommand:  "fail app6 1.0",
		RemoveCommand:   "remove app6 1.0",
		UpdateCommand:   "update app6 1.0",
		Order:           &two,
	}
	six := 6
	seven := 7
	new7 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app7",
		Version:         "1.0",
		InstallCommand:  "install app7 1.0",
		RemoveCommand:   "remove app7 1.0",
		UpdateCommand:   "update app7 1.0",
		Order:           &six,
	}
	new8 := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app8",
		Version:         "1.0",
		InstallCommand:  "install app8 1.0",
		RemoveCommand:   "remove app8 1.0",
		UpdateCommand:   "update app8 1.0",
		Order:           &seven,
	}
	incomingApps = append(incomingApps, &newFail6, &new7, &new8)
	cmdHandler = NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, actionPlan, statusMessage, executeError = executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assert.Equal(t, 5, len(actionPlan.orderedOperations), "5 orders expected")
	assert.Equal(t, 3, len(actionPlan.orderedOperations[2]), "3 operation of order 2")
	assert.Equal(t, 5, len(cmdHandler.Result), "5 total command executions are expected")
	assert.Equal(t, packageregistry.Failed, newReg[newFail6.ApplicationName].OngoingOperation, "We expect the app6 install to fail")
	assert.Equal(t, packageregistry.Skipped, newReg[new2.ApplicationName].OngoingOperation, "We expect the app2 update to be skipped")
	assert.Equal(t, packageregistry.Skipped, newReg[new7.ApplicationName].OngoingOperation, "We expect the app7 install to be skipped")
	assert.Equal(t, packageregistry.Skipped, newReg[new8.ApplicationName].OngoingOperation, "We expect the app8 install to be skipped")
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())

	// compare the status message and the new app registry
	packageOperationResults, ok = statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	successCountFromStatus := 0
	failCountFromStatus := 0
	skipCountFromStatus := 0
	for _, sMessage := range *packageOperationResults {
		app, exists := newReg[sMessage.PackageName]
		if sMessage.Operation == packageregistry.Remove.ToString() {
			assert.False(t, exists, "removed applications shouldn't be a part of the new application registry")
			continue
		}
		assert.Equal(t, app.ApplicationName, sMessage.PackageName)
		assert.Equal(t, app.Version, sMessage.AppVersion)
		if sMessage.Result == Success {
			assert.Equal(t, app.OngoingOperation, packageregistry.NoAction)
			successCountFromStatus++
		} else if strings.Contains(sMessage.Result, packageregistry.Skipped.ToString()) {
			assert.Equal(t, app.OngoingOperation, packageregistry.Skipped)
			skipCountFromStatus++

		} else if strings.Contains(sMessage.Result, errorString) {
			assert.Equal(t, app.OngoingOperation, packageregistry.Failed)
			failCountFromStatus++
		}
	}

	successCountFromRegistry := 0
	failCountFromRegistry := 0
	skipCountFromRegistry := 0

	for _, appInRegistry := range newReg {
		switch appInRegistry.OngoingOperation {
		case packageregistry.NoAction:
			successCountFromRegistry++
		case packageregistry.Failed:
			failCountFromRegistry++
		case packageregistry.Skipped:
			skipCountFromRegistry++
		}
	}

	assert.Equal(t, successCountFromRegistry, successCountFromStatus, "the success count should match")
	assert.Equal(t, failCountFromRegistry, failCountFromStatus, "the fail count should match")
	assert.Equal(t, skipCountFromRegistry, skipCountFromStatus, "the skip count should match")

	// we have tested the apps that were supposed to fail above, we need to assert that the remaining succeeded
	delete(newReg, newFail6.ApplicationName)
	delete(newReg, new2.ApplicationName)
	delete(newReg, new7.ApplicationName)
	delete(newReg, new8.ApplicationName)
	assert.Equal(t, 3, len(newReg))
	assertAllActionsSucceeded(t, newReg)
}

func TestSkippedPackagesAreCleanedUpWhenRemovedFromApplicationProfile(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	old1 := getVmAppPackageCurrent("app1", "1.0")
	old2 := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app2",
		Version:          "1.0",
		InstallCommand:   "install app2 1.0",
		RemoveCommand:    "remove app2 1.0",
		UpdateCommand:    "update app2 1.0",
		OngoingOperation: packageregistry.Skipped,
	}
	old3 := getVmAppPackageCurrent("app3", "1.0")

	existingApps := packageregistry.VMAppPackageCurrentCollection{&old1, &old2, &old3}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{}
	cmdHandler := NewCommandHandlerMock(mockCommandFailOnDemand)
	newReg, actionPlan, statusMessage, executeError := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assertAllActionsSucceeded(t, newReg)
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed())
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 3, len(actionPlan.unorderedImplicitUninstalls), "we are expecting 3 uninstall operations")
	assert.Equal(t, 2, len(cmdHandler.Result), "only 2 commands should be invoked")

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.Contains(t, *packageOperationResults, PackageOperationResult{Result: Success, Operation: packageregistry.Cleanup.ToString(), AppVersion: old2.Version, PackageName: old2.ApplicationName})

}

func executeActionPlan(t *testing.T,
	currentPackages packageregistry.VMAppPackageCurrentCollection,
	incomingPackages packageregistry.VMAppPackageIncomingCollection,
	cmdHandler commandhandler.ICommandHandler) (packageregistry.CurrentPackageRegistry, *ActionPlan, IResult, *ExecuteError) {

	currentReg := packageregistry.CurrentPackageRegistry{}
	currentReg.Populate(currentPackages)

	packageReg, err := packageregistry.New(el, environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(currentReg)
	assert.NoError(t, err)
	actionPlan := New(currentReg, incomingPackages, environment, new(NoopHostGaComminucator), el)

	el := logging.New(nil)
	he := getHandlerEnvironment()
	eem := extensionevents.New(el, he)

	executeError, statusMessage := actionPlan.Execute(packageReg, eem, cmdHandler)
	currentReg, err = packageReg.GetExistingPackages()
	assert.NoError(t, err)
	return currentReg, actionPlan, statusMessage, executeError
}

func getHandlerEnvironment() *handlerenv.HandlerEnvironment {
	return &handlerenv.HandlerEnvironment{
		HeartbeatFile: "",
		StatusFolder:  "",
		ConfigFolder:  "",
		LogFolder:     "",
		DataFolder:    "",
		EventsFolder:  "",
	}
}

func assertPackageRegistryHasBeenUpdatedProperly(t *testing.T, pkgReg packageregistry.CurrentPackageRegistry, incoming packageregistry.VMAppPackageIncomingCollection) {
	assert.Equal(t, len(incoming), len(pkgReg))
	for _, incomingVMApp := range incoming {
		vmApp, exists := pkgReg[incomingVMApp.ApplicationName]
		assert.True(t, exists)
		assert.EqualValues(t, incomingVMApp.ApplicationName, vmApp.ApplicationName)
		assert.EqualValues(t, incomingVMApp.Version, vmApp.Version)
		assert.EqualValues(t, incomingVMApp.InstallCommand, vmApp.InstallCommand)
		assert.EqualValues(t, incomingVMApp.RemoveCommand, vmApp.RemoveCommand)
		assert.EqualValues(t, incomingVMApp.UpdateCommand, vmApp.UpdateCommand)
		assert.EqualValues(t, incomingVMApp.DirectDownloadOnly, vmApp.DirectDownloadOnly)
	}
}

func assertAllActionsSucceeded(t *testing.T, pkgReg packageregistry.CurrentPackageRegistry) {
	for _, vmApp := range pkgReg {
		assert.Equal(t, packageregistry.NoAction, vmApp.OngoingOperation)
		assert.Contains(t, vmApp.Result, Success)
	}
}
