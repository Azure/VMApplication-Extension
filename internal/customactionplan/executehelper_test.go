package customactionplan

import (
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"path"
	"testing"
	"time"
)

var testdir = path.Join(".", "testdir")
var uploadDir = path.Join(testdir, "upload")

type mockHostGaCommunicator struct {
	pkgFileSourcePath    string
	configFileSourcePath string
	DownloadPackageCount int
	DownloadConfigCount  int
}

func (mockCommunicator *mockHostGaCommunicator) GetVMAppInfo(el *logging.ExtensionLogger, appName string) (*hostgacommunicator.VMAppMetadata, error) {
	return &hostgacommunicator.VMAppMetadata{}, nil
}

func (mockCommunicator *mockHostGaCommunicator) DownloadPackage(el *logging.ExtensionLogger, appName string, dst string) error {
	mockCommunicator.DownloadPackageCount++
	return copyFile(mockCommunicator.pkgFileSourcePath, dst)
}

func (mockCommunicator *mockHostGaCommunicator) DownloadConfig(el *logging.ExtensionLogger, appName string, dst string) error {
	mockCommunicator.DownloadConfigCount++
	return copyFile(mockCommunicator.configFileSourcePath, dst)
}

func copyFile(source, destination string) error {
	s, err := os.Open(source)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = io.Copy(d, s)
	return err
}

var handlerEnvironment = handlerenv.HandlerEnvironment{
	ConfigFolder: path.Join(testdir, "config"),
	StatusFolder: path.Join(testdir, "status"),
	LogFolder:    path.Join(testdir, "log"),
	DataFolder:   path.Join(testdir, "data"),
	EventsFolder: path.Join(testdir, "events"),
}

var actionPlan ActionPlan
var commandHandler = NewCommandHandlerMock(mockCommandExecutorNoError)
var extensionLogger = logging.New(nil)
var extensionEventManager = extensionevents.New(extensionLogger, &handlerEnvironment)
var vmAppPackageCurrent = packageregistry.VMAppPackageCurrent{
	ApplicationName:"test app",
	Version:"1.0.0",
	InstallCommand:"install",
	RemoveCommand:"remove",
	UpdateCommand:"update",
	ConfigExists:true,
	PackageFileName:"package",
	ConfigFileName:"config",

}

var packageRegistry packageregistry.IPackageRegistry

func initTest(t *testing.T){
	actionPlan = ActionPlan{
		environment :       &handlerEnvironment,
		logger:             extensionLogger,
	}
	err := os.MkdirAll(handlerEnvironment.ConfigFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.StatusFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.LogFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.DataFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.EventsFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)

	pkr, err := packageregistry.New(extLogger, &handlerEnvironment, 1 * time.Minute)
	assert.NoError(t, err)
	packageRegistry = pkr
}

func cleanTest(){
	packageRegistry.Close()
	os.RemoveAll(testdir)
}

func TestExecuteHelper(t *testing.T){
	initTest(t)
	defer cleanTest()
	act := action{vmAppPackageCurrent, ActionSetting{
		ActionName: "action1",
		ActionScript: "echo hello",
		Timestamp:"",
		Parameters: []ActionParameter{},
		TickCount: 1234567,
	}}
	err := actionPlan.executeHelper(commandHandler, ActionPackageRegistry{}, &act, extensionEventManager)
	assert.NoError(t, err)
	assertTickCountFileCorrect(t, act.Action.TickCount)

	act = action{vmAppPackageCurrent, ActionSetting{
		ActionName: "action2",
		ActionScript: "echo world",
		Timestamp:"",
		Parameters: []ActionParameter{},
		TickCount: 1234568,
	}}

	err = actionPlan.executeHelper(commandHandler, ActionPackageRegistry{}, &act, extensionEventManager)
	assert.NoError(t, err)
	assertTickCountFileCorrect(t, act.Action.TickCount)

}


