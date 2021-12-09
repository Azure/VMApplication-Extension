package customactionplan

import (
	"encoding/json"
	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/assert"
)

const LaunchedFromAnotherProcessEnvVariable = "LAUNCHED_FROM_ANOTHER_PROCESS"

var one = 1
var two = 2
var extLogger = logging.New(nil)

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
		os.Stderr.WriteString("could not create handler environment data directory")
		t.Fatal(err)
	}
}

func cleanupTest() {
	os.RemoveAll(testdir)
}

func TestSingleCustomAction(t *testing.T) {
	cleanupTest()
	initializeTest(t)
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
	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
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
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp.ApplicationName, Timestamp: "20210604T155300Z"})
}


func TestSingleCustomActionWithParameter(t *testing.T) {
	cleanupTest()
	initializeTest(t)
	action := []*VmAppSetting{
		{
			ApplicationName: "app1",
			Order:           &one,
			Actions: []*ActionSetting{
				{
					ActionName:   "action1",
					ActionScript: "echo %CustomAction_FOO%",
					Timestamp:    "20210604T155300Z",
					Parameters: []ActionParameter{
						{
							ParameterName:  "FOO",
							ParameterValue: "Hello World",
						},
					},
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
	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
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
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp.ApplicationName, Timestamp: "20210604T155300Z"})
}


func TestNoCustomAction(t *testing.T) {
	cleanupTest()
	initializeTest(t)
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
	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
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
	cleanupTest()
	initializeTest(t)
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
			Order: &two,
			Actions: []*ActionSetting{
				{
					ActionName: "action2",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
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

	newApp2 := packageregistry.VMAppPackageCurrent{
		ApplicationName: "app2",
		Version:         "1.0",
		InstallCommand:  "install app2",
		RemoveCommand:   "remove app2",
		UpdateCommand:   "update app2",
	}

	newRegistry := packageregistry.CurrentPackageRegistry{
		"app1": &newApp,
		"app2": &newApp2,
	}
	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
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
	assert.True(t, ok)
	assertTickCountFileCorrect(t, action[1].Actions[0].TickCount)
	assert.Len(t, *packageOperationResults, 2)
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action1", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[1], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action2", AppVersion: "1.0", PackageName: action[1].ApplicationName, Timestamp: "20210604T155300Z"})
}

func TestDoubleCustomActionNonexistentApp(t *testing.T) {
	cleanupTest()
	initializeTest(t)
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
	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
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
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp.ApplicationName, Timestamp: "20210604T155300Z"})
}

func TestDoubleCustomActionOldTickCount(t *testing.T) {
	cleanupTest()
	initializeTest(t)
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
	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
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
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp1.ApplicationName, Timestamp: "20210604T155300Z"})
}

func TestMaxCustomActions(t *testing.T) {
	cleanupTest()
	initializeTest(t)
	action := []*VmAppSetting {
		{
			ApplicationName: "app1",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action16",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193143,
				},
				{
					ActionName: "action15",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193141,
				},
				{
					ActionName: "action14",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193139,
				},
				{
					ActionName: "action13",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193137,
				},
				{
					ActionName: "action12",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193135,
				},
				{
					ActionName: "action11",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193133,
				},
				{
					ActionName: "action10",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193131,
				},
				{
					ActionName: "action9",
					ActionScript: "echo hello",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193129,
				},
				{
					ActionName: "action8",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193127,
				},
				{
					ActionName: "action7",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193125,
				},
				{
					ActionName: "action6",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193123,
				},
				{
					ActionName: "action5",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193121,
				},
				{
					ActionName: "action4",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193119,
				},
				{
					ActionName: "action3",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193117,
				},
				{
					ActionName: "action2",
					ActionScript: "echo world",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193115,
				},
				{
					ActionName: "action1",
					ActionScript: "echo hello",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{},
					TickCount: 10193114,
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
	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
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
	assert.True(t, ok)
	assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
	assert.Len(t, *packageOperationResults, 16)
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action1", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[1], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action2", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[2], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action3", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[3], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action4", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[4], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action5", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[5], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action6", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[6], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action7", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[7], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action8", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[8], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action9", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[9], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action10", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[10], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action11", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[11], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action12", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[12], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action13", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[13], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action14", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[14], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action15", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
	assert.EqualValues(t, (*packageOperationResults)[15], actionplan.PackageOperationResult{Result: actionplan.Success, Operation: "action16", AppVersion: "1.0", PackageName: action[0].ApplicationName, Timestamp: "20210604T155300Z"})
}

func executeActionPlan(t *testing.T,
	settings []*VmAppSetting,
	appPackage packageregistry.CurrentPackageRegistry,
	cmdHandler commandhandler.ICommandHandlerWithEnvVariables) ( *ActionPlan, actionplan.IResult) {

	actionPlan, err := New(settings, appPackage, environment, extLogger)
	assert.NoError(t, err)

	extLogger := logging.New(nil)
	handlerEnv := getHandlerEnvironment()
	extEventManager := extensionevents.New(extLogger, handlerEnv)

	vmAppResult := actionplan.PackageOperationResults{}

 	_, statusMessage := actionPlan.Execute(extEventManager, cmdHandler, &vmAppResult)
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
  