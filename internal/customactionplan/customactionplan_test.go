package customactionplan

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"io/ioutil"
	"os"
	"path"
	"strconv"
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

func (commandHandlerMock *CommandHandlerMock) ExecuteWithEnvVariables(command string, workingDir, logDir string, waitForCompletion bool, el *logging.ExtensionLogger,  params *map[string]string ) (returnCode int, err error) {
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

func TestSingleCustomAction(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	action := []*VmAppSetting {
		{
			ApplicationName: "app1",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action1",
					ActionScript: "echo hello",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193113,
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
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(newRegistry)
	assert.NoError(t, err)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	appPackage, err := packageReg.GetExistingPackages()

	_, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)

	packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
	assert.True(t, ok)
	assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp.ApplicationName, Timestamp: "20210604T155300Z"})
}

func TestNoCustomAction(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	action := []*VmAppSetting {
		{
			ApplicationName: "app1",
			Order: &one,
			Actions: []*ActionSetting{},
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
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(newRegistry)
	assert.NoError(t, err)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	appPackage, err := packageReg.GetExistingPackages()
	_, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)

	_, ok := statusMessage.(*actionplan.PackageOperationResults)
	assert.True(t, ok)
}

func TestDoubleCustomAction(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	action := []*VmAppSetting {
		{
			ApplicationName: "app1",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action2",
					ActionScript: "echo world",
					Timestamp: "20210604T155330Z",
					Parameters: []ActionParameter{},
					TickCount: 10193115,
				},
				{
					ActionName: "action1",
					ActionScript: "echo hello",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193113,
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
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(newRegistry)
	assert.NoError(t, err)

	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	appPackage, err := packageReg.GetExistingPackages()

	actionPlan, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)
	assertActionOrder(t, actionPlan)

	packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
	fmt.Println(packageOperationResults)
	assert.True(t, ok)
	assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
	assert.Len(t, *packageOperationResults, 2)
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: Success, Operation: "action1", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[1], actionplan.PackageOperationResult{Result: Success, Operation: "action2", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155330Z"})
}

func TestDoubleCustomActionNonexistantApp(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	action := []*VmAppSetting {
		{
			ApplicationName: "app1",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action1",
					ActionScript: "echo hello",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193113,
				},
			},
		},
		{
			ApplicationName: "app2",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action2",
					ActionScript: "echo world",
					Timestamp: "20210604T155330Z",
					Parameters: []ActionParameter{},
					TickCount: 10193115,
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
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(newRegistry)
	assert.NoError(t, err)

	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	appPackage, err := packageReg.GetExistingPackages()

	actionPlan, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)
	assert.Len(t, actionPlan.sortedOrder, 1)

	packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
	assert.True(t, ok)
	assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp.ApplicationName, Timestamp: "20210604T155300Z"})
}

func TestDoubleCustomActionOldTickCount(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()

	action := []*VmAppSetting {
		{
			ApplicationName: "app1",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action1",
					ActionScript: "echo hello",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193113,
				},
			},
		},
		{
			ApplicationName: "app2",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action2",
					ActionScript: "echo world",
					Timestamp: "20210604T155330Z",
					Parameters: []ActionParameter{},
					TickCount: 10193110,
				},
			},
		},
	}

	tickCountFile := path.Join(environment.ConfigFolder, "tickCount")

	os.Create(tickCountFile)
	bytes, _ := json.Marshal(uint64(10193112))

	ioutil.WriteFile(tickCountFile, bytes, constants.FilePermissions_UserOnly_ReadWrite)
	newApp1 := packageregistry.VMAppPackageCurrent{
		ApplicationName: "app1",
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	newApp2 := packageregistry.VMAppPackageCurrent{
		ApplicationName: "app2",
		Version:         "1.0",
		InstallCommand:  "install app2",
		RemoveCommand:   "remove app2",
		UpdateCommand:   "update app2",
	}
	newRegistry := packageregistry.CurrentPackageRegistry{
		"app1": &newApp1,
		"app2": &newApp2,
	}
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(newRegistry)
	assert.NoError(t, err)

	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	appPackage, err := packageReg.GetExistingPackages()

	actionPlan, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)
	assert.Len(t, actionPlan.sortedOrder, 1)

	packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
	assert.True(t, ok)
	assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp1.ApplicationName, Timestamp: "20210604T155300Z"})
}

func executeActionPlan(t *testing.T,
	settings []*VmAppSetting,
	appPackage packageregistry.CurrentPackageRegistry,
	cmdHandler commandhandler.ICommandHandlerWithEnvVariables) ( *ActionPlan, actionplan.IResult) {

	actionPlan, err := New(settings, appPackage, environment, el)
	assert.NoError(t, err)

	el := logging.New(nil)
	he := getHandlerEnvironment()
	eem := extensionevents.New(el, he)

	vmAppResult := actionplan.PackageOperationResults{}

	_, statusMessage := actionPlan.Execute(eem, cmdHandler, &vmAppResult)
	assert.NoError(t, err)
	return actionPlan, statusMessage
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

func assertTickCountFileCorrect(t *testing.T, tickCount uint64) {
	tc := strconv.FormatUint(tickCount, 10)
	tickCountFile := path.Join(environment.ConfigFolder, "tickCount")
	assert.FileExists(t, tickCountFile)
	file, err := ioutil.ReadFile(tickCountFile)
	assert.NoError(t, err)
	assert.Contains(t, string(file), tc)
}

func assertActionOrder(t *testing.T, actionPlan *ActionPlan) {
	currTickCount :=uint64(0)
	for _, act := range actionPlan.sortedOrder {
		assert.Less(t, currTickCount, act.Action.TickCount)
		currTickCount = act.Action.TickCount
	}
}
