package actionplan

import (
	"github.com/Azure/VMApplication-Extension/VmApp/constants"
	"github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/cmd"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"
)

var one = 1
var emptyPackageRegistry = packageregistry.CurrentPackageRegistry{}
var app1 = &packageregistry.VMAppPackageIncoming{
	ApplicationName: "app1",
	Order:           &one,
	Version:         "1.0",
	InstallCommand:  "echo install",
	RemoveCommand:   "echo remove",
	UpdateCommand:   "echo update",
}

var app2 = &packageregistry.VMAppPackageIncoming{
	ApplicationName: "app1",
	Order:           nil,
	Version:         "1.0",
	InstallCommand:  "echo install",
	RemoveCommand:   "echo remove",
	UpdateCommand:   "echo update",
}

type CommandHandlerMock struct {
	CommandsInvoked []string
	Executor        func(string, string) (int, error)
}

func NewCommandHandlerMock(executor func(string, string) (int, error)) (*CommandHandlerMock) {
	return &CommandHandlerMock{CommandsInvoked: make([]string, 0), Executor: executor}
}

func (commandHandlerMock *CommandHandlerMock) Execute(command string, workingDir string) (returnCode int, err error) {
	commandHandlerMock.CommandsInvoked = append(commandHandlerMock.CommandsInvoked, command)
	return commandHandlerMock.Executor(command, workingDir)
}

var mockCommandExecutorNoError = func(string, string) (int, error) {
	return 0, nil
}

var mockCommandExecutorNonZeroReturnCode = func(string, string) (int, error) {
	return rand.Intn(255) + 1, nil // returns an exit code between 1 and 255
}

var environment = &vmextensionhelper.HandlerEnvironment{
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
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(emptyPackageRegistry)
	assert.NoError(t, err)
	incomingCollection := packageregistry.VMAppPackageIncomingCollection{app1}
	actionPlan, err := New(emptyPackageRegistry, incomingCollection, environment)
	assert.NoError(t, err)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	err = actionPlan.Execute(packageReg, cmdHandler)
	assert.NoError(t, err, "execution of actionPlan should succeed")
	assert.EqualValues(t, app1.InstallCommand, cmdHandler.CommandsInvoked[0], "Install command be invoked")
	newReg, err := packageReg.GetExistingPackages()
	assert.NoError(t, err)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingCollection)
	assertAllActionsSucceeded(t, newReg)
}

func TestSingleInstallWithoutOrder(t *testing.T){
	initializeTest(t)
	defer cleanupTest()
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(emptyPackageRegistry)
	assert.NoError(t, err)
	incomingCollection := packageregistry.VMAppPackageIncomingCollection{app2}
	actionPlan, err := New(emptyPackageRegistry, incomingCollection, environment)
	assert.NoError(t, err)
	cmdHandler := cmd.NewCommandHandler()
	err = actionPlan.Execute(packageReg, cmdHandler)
	assert.NoError(t, err, "execution of actionPlan should succeed")
	newReg, err := packageReg.GetExistingPackages()
	assert.NoError(t, err)
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingCollection)
	assertAllActionsSucceeded(t, newReg)
}

func TestSingleRemove(t *testing.T){
	initializeTest(t)
	incomingCollection := packageregistry.VMAppPackageIncomingCollection{}
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}

	currentVmApp := packageregistry.VMAppPackageIncomingToVmAppPackageCurrent(app1)
	currentCollection := packageregistry.VMAppPackageCurrentCollection{currentVmApp}
	currentReg := packageregistry.CurrentPackageRegistry{}
	currentReg.Populate(currentCollection)
	err = packageReg.WriteToDisk(currentReg)
	assert.NoError(t, err)

	actionPlan, err := New(currentReg, incomingCollection, environment)
	assert.NoError(t, err)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	err = actionPlan.Execute(packageReg, cmdHandler)
	assert.NoError(t, err, "execution of actionPlan should succeed")
	assert.EqualValues(t, app1.RemoveCommand, cmdHandler.CommandsInvoked[0], "Remove command should be invoked")
	currentReg, err = packageReg.GetExistingPackages()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(currentReg)) // the current registry should have no applications
	if err == nil {
		cleanupTest()
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
		assert.EqualValues(t, incomingVMApp.PackageLocation, vmApp.PackageLocation)
		assert.EqualValues(t, incomingVMApp.ConfigurationLocation, vmApp.ConfigurationLocation)
		assert.EqualValues(t, incomingVMApp.DirectDownloadOnly, vmApp.DirectDownloadOnly)
	}
}

func assertAllActionsSucceeded(t *testing.T, pkgReg packageregistry.CurrentPackageRegistry) {
	for _, vmApp := range pkgReg {
		assert.Equal(t, packageregistry.NoAction, vmApp.OngoingOperation)
	}
}
