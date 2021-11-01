package actionplan

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/assert"
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

func newMockHostGaCommunicator(packageFileContent, configFileContent []byte) (*mockHostGaCommunicator, error) {
	pkgFile := path.Join(uploadDir, "pkgFile")
	configFile := path.Join(uploadDir, "configFile")
	err := os.MkdirAll(uploadDir, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(pkgFile, packageFileContent, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(configFile, configFileContent, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		return nil, err
	}
	return &mockHostGaCommunicator{pkgFileSourcePath: pkgFile, configFileSourcePath: configFile, DownloadConfigCount: 0, DownloadPackageCount: 0}, nil
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
	ApplicationName: "test app",
	Version:         "1.0.0",
	InstallCommand:  "install",
	RemoveCommand:   "remove",
	UpdateCommand:   "update",
	ConfigExists:    true,
	PackageFileName: "package",
	ConfigFileName:  "config",
}

var packageRegistry packageregistry.IPackageRegistry

var mhgCommunicator *mockHostGaCommunicator

func initTest(t *testing.T) {
	mhc, err := newMockHostGaCommunicator([]byte("package File"), []byte("config file"))
	assert.NoError(t, err, "mockHostGaCommunicator initialization should succeed")
	actionPlan = ActionPlan{
		environment:        &handlerEnvironment,
		logger:             extensionLogger,
		hostGaCommunicator: mhc,
	}
	mhgCommunicator = mhc
	err = os.MkdirAll(handlerEnvironment.ConfigFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.StatusFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.LogFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.DataFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.MkdirAll(handlerEnvironment.EventsFolder, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)

	pkr, err := packageregistry.New(extensionLogger, &handlerEnvironment, 1*time.Minute)
	assert.NoError(t, err)
	packageRegistry = pkr
}

func cleanTest() {
	packageRegistry.Close()
	os.RemoveAll(testdir)
}

func TestExecuteHelper(t *testing.T) {
	initTest(t)
	defer cleanTest()
	action := action{vmAppPackageCurrent, packageregistry.Install}
	err := actionPlan.executeHelper(packageRegistry, commandHandler, packageregistry.CurrentPackageRegistry{}, &action, extensionEventManager)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, mhgCommunicator.DownloadPackageCount, "download package count should be 1")
	assert.EqualValues(t, 1, mhgCommunicator.DownloadConfigCount, "download config count should be 1")

	action.actionToPerform = packageregistry.Remove
	err = actionPlan.executeHelper(packageRegistry, commandHandler, packageregistry.CurrentPackageRegistry{}, &action, extensionEventManager)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, mhgCommunicator.DownloadPackageCount, "download package count should be 1")
	assert.EqualValues(t, 1, mhgCommunicator.DownloadConfigCount, "download config count should be 1")

	assert.EqualValues(t, vmAppPackageCurrent.InstallCommand, commandHandler.Result[0].command, "1st command should be install")
	assert.EqualValues(t, vmAppPackageCurrent.RemoveCommand, commandHandler.Result[1].command, "2nd command should be remove")
	assert.Equal(t, 2, len(commandHandler.Result), "only 2 commands should be executed")
	_, err = os.Stat(vmAppPackageCurrent.DownloadDir)
	assert.Error(t, err, "downloadDir should be deleted")
	_, ok := err.(*os.PathError)
	assert.True(t, ok, "downloadDir should be deleted")
}

func TestChecksum(t *testing.T) {
	initTest(t)
	defer cleanTest()
	checksum, err := getMD5CheckSum(mhgCommunicator.pkgFileSourcePath)
	assert.NotNil(t, checksum, "checksum should not be nil")
	assert.NoError(t, err, "we should be able to get checksum")
	checksumMatch, err := verifyMD5CheckSum(mhgCommunicator.pkgFileSourcePath, checksum)
	assert.NoError(t, err, "verify checksum should not throw error")
	assert.True(t, checksumMatch, "checksum should match")

	checksumMatch, err = verifyMD5CheckSum(mhgCommunicator.configFileSourcePath, checksum)
	assert.NoError(t, err, "verify checksum should not throw error")
	assert.False(t, checksumMatch, "checksum should not match")
}
