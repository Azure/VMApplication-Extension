package actionplan

import (
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

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

func (commandHandlerMock *CommandHandlerMock) Execute(command string, workingDir string) (returnCode int, err error) {
	retCode, err := commandHandlerMock.Executor(command, workingDir)
	commandHandlerMock.Result = append(commandHandlerMock.Result, commandResult{command, retCode, err})
	return
}

var mockCommandExecutorNoError CommandExecutor = func(string, string) (int, error) {
	return 0, nil
}

var mockCommandFailOnDemand CommandExecutor = func(command string, workingDir string) (int, error) {
	if strings.HasPrefix(command, "fail") {
		return -1, errors.Errorf("command failed as expected")
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
	DataFolder:   path.Join(".", "testdir", "data"),
	ConfigFolder: path.Join(".", "testdir", "config"),
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
	os.RemoveAll(environment.ConfigFolder)
	os.RemoveAll(environment.DataFolder)
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
	newReg, _ := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)
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
	newReg, _ := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)
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
	newReg, _ := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, 0, len(newReg)) // the current registry should have no applications
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)
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
	newReg, actionPlan := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 1, len(actionPlan.unorderedOperations[0]), "there must be 1 dependent actions")
	assert.Equal(t, 1, len(cmdHandler.Result), "1 command must be invoked")
	assert.Equal(t, newVersion.UpdateCommand, cmdHandler.Result[0].command, "the update method for the new app version must be called")

	// test the same for ordered actions
	newVersion.Order = &one
	cmdHandler = NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan = executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.orderedOperations), "there must be 1 ordered operation")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[one]), "there must be only one set of dependent actions for order == 1")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[one][0]), "there must be 1 dependent action")
	assert.Equal(t, 1, len(cmdHandler.Result), "1 command must be invoked")
	assert.Equal(t, newVersion.UpdateCommand, cmdHandler.Result[0].command, "the update method for the new app version must be called")
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
	newReg, actionPlan := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 2, len(actionPlan.unorderedOperations[0]), "there must be 2 dependent actions")
	assert.Equal(t, 2, len(cmdHandler.Result), "2 commands must be invoked")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, newVersion.InstallCommand, cmdHandler.Result[1].command, "the install method for the new app version must be called")

	// test the same for ordered actions
	newVersion.Order = &one
	cmdHandler = NewCommandHandlerMock(mockCommandExecutorNoError)
	newReg, actionPlan = executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assertAllActionsSucceeded(t, newReg)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assert.Equal(t, 1, len(actionPlan.orderedOperations), "there must be only one ordered operation")
	assert.Equal(t, 1, len(actionPlan.orderedOperations[one]), "there must be only one set of dependent actions for order == 1")
	assert.Equal(t, 2, len(actionPlan.orderedOperations[one][0]), "there must be 2 dependent actions")
	assert.Equal(t, 2, len(cmdHandler.Result), "2 commands must be invoked")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, newVersion.InstallCommand, cmdHandler.Result[1].command, "the install method for the new app version must be called")
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
	newReg, actionPlan := executeActionPlan(t, existingApps, incomingApps, cmdHandler)

	assert.Equal(t, 1, len(actionPlan.unorderedOperations), "there must be 1 unordered operation")
	assert.Equal(t, 2, len(actionPlan.unorderedOperations[0]), "there must be 2 dependent actions")
	assert.Equal(t, 1, len(cmdHandler.Result), "only one command should have been executed")
	assert.Equal(t, oldVersion.RemoveCommand, cmdHandler.Result[0].command, "the remove method for the old app version must be called")
	assert.Equal(t, packageregistry.Failed, newReg[oldVersion.ApplicationName].OngoingOperation, "the package status should be failed")
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
	newReg, actionPlan := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
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
	newReg, actionPlan = executeActionPlan(t, existingApps, incomingApps, cmdHandler)
	assert.Equal(t, 5, len(actionPlan.orderedOperations), "5 orders expected")
	assert.Equal(t, 3, len(actionPlan.orderedOperations[2]), "3 operation of order 2")
	assert.Equal(t, 5, len(cmdHandler.Result), "5 total command executions are expected")
	assert.Equal(t, packageregistry.Failed, newReg[newFail6.ApplicationName].OngoingOperation, "We expect the app6 install to fail")
	assert.Equal(t, packageregistry.Skipped, newReg[new2.ApplicationName].OngoingOperation, "We expect the app2 update to be skipped")
	assert.Equal(t, packageregistry.Skipped, newReg[new7.ApplicationName].OngoingOperation, "We expect the app7 install to be skipped")
	assert.Equal(t, packageregistry.Skipped, newReg[new8.ApplicationName].OngoingOperation, "We expect the app8 install to be skipped")

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
	cmdHandler commandhandler.ICommandHandler) (packageregistry.CurrentPackageRegistry, *ActionPlan) {

	currentReg := packageregistry.CurrentPackageRegistry{}
	currentReg.Populate(currentPackages)

	packageReg, err := packageregistry.New(el, environment, time.Second)
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

	actionPlan.Execute(packageReg, eem, cmdHandler)
	currentReg, err = packageReg.GetExistingPackages()
	assert.NoError(t, err)
	return currentReg, actionPlan
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
