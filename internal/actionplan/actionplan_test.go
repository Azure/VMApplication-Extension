package actionplan

import (
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

// On windows, to actually run this test, you have to CTRL-C the process running the test then verify the package registry file
// in the testdir manually. Go cannot send SIGTERM to a process running in windows, but it can catch the interrupts like
// CTRL_CLOSE_EVENT, CTRL_LOGOFF_EVENT or CTRL_SHUTDOWN_EVENT and do the cleanup before exiting
// the test wouldn't fail if you fail to terminate it, but it wouldn't exercise the code that is meant to deal with reboots after installing
// a VMApp

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
	newReg, _, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newApp.Version, PackageName: newApp.ApplicationName})
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
	newReg, _, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newApp.Version, PackageName: newApp.ApplicationName})

}

func TestSingleRemove(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	currentVmApp := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "1.0",
		InstallCommand:   "install app1",
		RemoveCommand:    "remove app1",
		UpdateCommand:    "update app1",
		OngoingOperation: packageregistry.NoAction,
	}

	existingApps := packageregistry.VMAppPackageCurrentCollection{&currentVmApp}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, _, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, 0, len(newReg)) // the current registry should have no applications
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Remove.ToString(), AppVersion: currentVmApp.Version, PackageName: currentVmApp.ApplicationName})
}

func TestUpdateCommandIsCalledWhenPresent(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	oldVersion := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "1.0",
		InstallCommand:   "install app1 1.0",
		RemoveCommand:    "remove app1 1.0",
		UpdateCommand:    "update app1 1.0",
		OngoingOperation: packageregistry.NoAction,
	}
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
	newReg, actionPlan, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
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
	newReg, actionPlan, statusMessage = executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
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
	oldVersion := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "1.0",
		InstallCommand:   "install app1 1.0",
		RemoveCommand:    "remove app1 1.0",
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
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 2, len(actionPlan.unorderedOperations[0]), "there must be 2 dependent actions")
	assert.Equal(t, 2, len(cmdHandler.Result), "2 commands must be invoked")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, newVersion.InstallCommand, cmdHandler.Result[1].command, "the install method for the new app version must be called")

	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Remove.ToString(), AppVersion: oldVersion.Version, PackageName: oldVersion.ApplicationName})
	assert.EqualValues(t, (*packageOperationResults)[1], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newVersion.Version, PackageName: newVersion.ApplicationName})

	// test the same for ordered actions
	newVersion.Order = &one
	cmdHandler = NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan, statusMessage = executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.orderedOperations), "there must be only one ordered operation")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[one]), "there must be only one set of dependent actions for order == 1")
	assert.Equal(t, 2, len(actionPlan.orderedOperations[one][0]), "there must be 2 dependent actions")
	assert.Equal(t, 2, len(cmdHandler.Result), "2 commands must be invoked")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, newVersion.InstallCommand, cmdHandler.Result[1].command, "the install method for the new app version must be called")

	packageOperationResults, ok = statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Remove.ToString(), AppVersion: oldVersion.Version, PackageName: oldVersion.ApplicationName})
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
	newReg, actionPlan, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 2, len(actionPlan.unorderedOperations[0]), "there must be 2 dependent actions")
	assert.Equal(t, 1, len(cmdHandler.Result), "only one command should have been executed")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, packageregistry.Failed, newReg[oldVersion.ApplicationName].OngoingOperation, "the package status should be failed")
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.Contains(t, (*packageOperationResults)[0].Result, errorString)
}

func TestOrderIsMaintainedAndHigherOrderOperationsAreSkippedOnFailure(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	old1 := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "1.0",
		InstallCommand:   "install app1 1.0",
		RemoveCommand:    "remove app1 1.0",
		UpdateCommand:    "update app1 1.0",
		OngoingOperation: packageregistry.NoAction,
	}
	old2 := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app2",
		Version:          "1.0",
		InstallCommand:   "install app2 1.0",
		RemoveCommand:    "remove app2 1.0",
		UpdateCommand:    "update app2 1.0",
		OngoingOperation: packageregistry.NoAction,
	}
	old3 := packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app3",
		Version:          "1.0",
		InstallCommand:   "install app3 1.0",
		RemoveCommand:    "remove app3 1.0",
		UpdateCommand:    "update app3 1.0",
		OngoingOperation: packageregistry.NoAction,
	}
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
	newReg, actionPlan, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assertAllActionsSucceeded(t, newReg)
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
	newReg, actionPlan, statusMessage = executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assert.Equal(t, 5, len(actionPlan.orderedOperations), "5 orders expected")
	assert.Equal(t, 3, len(actionPlan.orderedOperations[2]), "3 operation of order 2")
	assert.Equal(t, 5, len(cmdHandler.Result), "5 total command executions are expected")
	assert.Equal(t, packageregistry.Failed, newReg[newFail6.ApplicationName].OngoingOperation, "We expect the app6 install to fail")
	assert.Equal(t, packageregistry.Skipped, newReg[new2.ApplicationName].OngoingOperation, "We expect the app2 update to be skipped")
	assert.Equal(t, packageregistry.Skipped, newReg[new7.ApplicationName].OngoingOperation, "We expect the app7 install to be skipped")
	assert.Equal(t, packageregistry.Skipped, newReg[new8.ApplicationName].OngoingOperation, "We expect the app8 install to be skipped")

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

func executeActionPlan(t *testing.T,
	currentPackages packageregistry.VMAppPackageCurrentCollection,
	incomingPackages packageregistry.VMAppPackageIncomingCollection,
	cmdHandler commandhandler.ICommandHandler) (packageregistry.CurrentPackageRegistry, *ActionPlan, IStatusMessage) {

	currentReg := packageregistry.CurrentPackageRegistry{}
	currentReg.Populate(currentPackages)

	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(currentReg)
	assert.NoError(t, err)
	actionPlan, err := New(currentReg, incomingPackages, environment, new(NoopHostGaComminucator), el)
	assert.NoError(t, err)

	el := logging.New(nil)
	he := getHandlerEnvironment()
	eem := extensionevents.New(el, he)

	_, statusMessage := actionPlan.Execute(packageReg, eem, cmdHandler)
	currentReg, err = packageReg.GetExistingPackages()
	assert.NoError(t, err)
	return currentReg, actionPlan, statusMessage
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
	}
}
