// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package actionplan

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/exithelper"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/assert"
)

var testdir = path.Join(".", "testdir")
var uploadDir = path.Join(testdir, "upload")

type mockHostGaCommunicator struct {
	pkgFileSourcePath       string
	configFileSourcePath    string
	DownloadPackageCount    int
	DownloadConfigCount     int
	packageStrictVersioning []bool
	configStrictVersioning  []bool
}

func (mockCommunicator *mockHostGaCommunicator) GetVMAppInfo(el *logging.ExtensionLogger, appName string, version string) (*hostgacommunicator.VMAppMetadata, error) {
	return &hostgacommunicator.VMAppMetadata{}, nil
}

func (mockCommunicator *mockHostGaCommunicator) DownloadPackage(el *logging.ExtensionLogger, appName string, version string, dst string, useStrictVersioning bool) error {
	mockCommunicator.DownloadPackageCount++
	mockCommunicator.packageStrictVersioning = append(mockCommunicator.packageStrictVersioning, useStrictVersioning)
	return copyFile(mockCommunicator.pkgFileSourcePath, dst)
}

func (mockCommunicator *mockHostGaCommunicator) DownloadConfig(el *logging.ExtensionLogger, appName string, version string, dst string, useStrictVersioning bool) error {
	mockCommunicator.DownloadConfigCount++
	mockCommunicator.configStrictVersioning = append(mockCommunicator.configStrictVersioning, useStrictVersioning)
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
	err = os.WriteFile(pkgFile, packageFileContent, constants.FilePermissions_UserOnly_ReadWriteExecute)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(configFile, configFileContent, constants.FilePermissions_UserOnly_ReadWriteExecute)
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

var vmAppPackageDeleted = packageregistry.VMAppPackageCurrent{
	ApplicationName: "test app",
	Version:         "1.0.0",
	InstallCommand:  "install",
	RemoveCommand:   "remove",
	UpdateCommand:   "update",
	ConfigExists:    true,
	PackageFileName: "package",
	ConfigFileName:  "config",
	IsDeleted:       true,
}

type recordingExitHelper struct {
	called bool
	code   int
}

func (helper *recordingExitHelper) Exit(code int) {
	helper.called = true
	helper.code = code
}

func TestCommandTerminationBeforeRebootSignal(t *testing.T) {
	for _, test := range []struct {
		name        string
		retCode     int
		stopping    bool
		shutdownErr error
		rerun       bool
		wantReboot  bool
	}{
		{name: "shutdown without rerun", retCode: -1, stopping: true, wantReboot: true},
		{name: "shutdown with rerun", retCode: -1, stopping: true, rerun: true, wantReboot: true},
		{name: "ordinary signal failure", retCode: -1},
		{name: "unavailable shutdown state", retCode: -1, shutdownErr: errors.New("systemctl unavailable")},
		{name: "ordinary command failure during shutdown", retCode: 1, stopping: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			initTest(t)
			defer cleanTest()
			previousCheck, previousExiter := systemShutdownInProgress, exithelper.Exiter
			defer func() {
				systemShutdownInProgress, exithelper.Exiter = previousCheck, previousExiter
			}()
			checked := false
			systemShutdownInProgress = func() (bool, error) {
				checked = true
				return test.stopping, test.shutdownErr
			}
			exiter := &recordingExitHelper{}
			exithelper.Exiter = exiter
			pkg := vmAppPackageCurrent
			if test.rerun {
				pkg.RebootBehavior = packageregistry.Rerun
			}
			registry := make(packageregistry.CurrentPackageRegistry)
			cmd := NewCommandHandlerMock(func(string, string) (int, error) {
				return test.retCode, errors.New("command terminated")
			})
			err := actionPlan.executeHelper(packageRegistry, cmd, registry,
				&action{vmAppPackage: pkg, actionToPerform: packageregistry.Install}, extensionEventManager)
			assert.Equal(t, test.retCode < 0, checked)
			assert.Equal(t, test.wantReboot, exiter.called)
			persisted, readErr := packageRegistry.GetExistingPackages()
			assert.NoError(t, readErr)
			current := persisted[pkg.ApplicationName]
			if test.wantReboot {
				assert.NoError(t, err)
				assert.Equal(t, 0, exiter.code)
				assert.Contains(t, current.Result, "Reboot detected during 'Install' operation")
				if test.rerun {
					assert.Equal(t, packageregistry.Install, current.OngoingOperation)
					assert.Equal(t, 1, current.NumRebootsOccurred)
				} else {
					assert.Equal(t, packageregistry.NoAction, current.OngoingOperation)
					assert.Equal(t, 0, current.NumRebootsOccurred)
				}
			} else {
				assert.Error(t, err)
				assert.Equal(t, packageregistry.Failed, current.OngoingOperation)
			}
		})
	}
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
	downloadDir := vmAppPackageCurrent.GetWorkingDirectory(&handlerEnvironment)
	action := action{vmAppPackageCurrent, false, packageregistry.Install}
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
	_, err = os.Stat(downloadDir)
	assert.True(t, os.IsNotExist(err), "downloadDir should be deleted")
}

func TestExecuteHelper_FailedRemoveForUpdatePreservesOutgoingPackage(t *testing.T) {
	initTest(t)
	defer cleanTest()

	outgoing := vmAppPackageCurrent
	outgoing.DownloadDir = outgoing.GetWorkingDirectory(&handlerEnvironment)
	assert.NoError(t, os.MkdirAll(outgoing.DownloadDir, constants.FilePermissions_UserOnly_ReadWriteExecute))
	packagePath := path.Join(outgoing.DownloadDir, outgoing.PackageFileName)
	configPath := path.Join(outgoing.DownloadDir, outgoing.ConfigFileName)
	assert.NoError(t, os.WriteFile(packagePath, []byte("cached package"), constants.FilePermissions_UserOnly_ReadWriteExecute))
	assert.NoError(t, os.WriteFile(configPath, []byte("cached config"), constants.FilePermissions_UserOnly_ReadWriteExecute))
	command := NewCommandHandlerMock(func(string, string) (int, error) { return 1, nil })
	registry := packageregistry.CurrentPackageRegistry{}
	act := action{outgoing, false, packageregistry.RemoveForUpdate}

	for attempt := 0; attempt < 2; attempt++ {
		err := actionPlan.executeHelper(packageRegistry, command, registry, &act, extensionEventManager)
		assert.Error(t, err)
		packageBytes, packageErr := os.ReadFile(packagePath)
		assert.NoError(t, packageErr)
		assert.Equal(t, "cached package", string(packageBytes))
		configBytes, configErr := os.ReadFile(configPath)
		assert.NoError(t, configErr)
		assert.Equal(t, "cached config", string(configBytes))
		assert.Equal(t, packageregistry.Failed, registry[outgoing.ApplicationName].OngoingOperation)
		assert.Equal(t, outgoing.Version, registry[outgoing.ApplicationName].Version)
	}
	assert.Len(t, command.Result, 2)
	assert.Zero(t, mhgCommunicator.DownloadPackageCount)
	assert.Zero(t, mhgCommunicator.DownloadConfigCount)
}

func TestExecuteHelper_RemoveForUpdateDoesNotDownloadOutgoingPackage(t *testing.T) {
	initTest(t)
	defer cleanTest()

	outgoingPackage := vmAppPackageCurrent
	outgoingPackage.DownloadDir = outgoingPackage.GetWorkingDirectory(&handlerEnvironment)
	err := os.MkdirAll(outgoingPackage.DownloadDir, constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.WriteFile(path.Join(outgoingPackage.DownloadDir, outgoingPackage.PackageFileName), []byte("cached outgoing package"), constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	err = os.WriteFile(path.Join(outgoingPackage.DownloadDir, outgoingPackage.ConfigFileName), []byte("cached outgoing config"), constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err)
	outgoingPackage.PackageFileMD5Checksum, err = getMD5CheckSum(path.Join(outgoingPackage.DownloadDir, outgoingPackage.PackageFileName))
	assert.NoError(t, err)
	outgoingPackage.ConfigFileMD5Checksum, err = getMD5CheckSum(path.Join(outgoingPackage.DownloadDir, outgoingPackage.ConfigFileName))
	assert.NoError(t, err)

	cachedFilesFound := false
	removeCommandHandler := NewCommandHandlerMock(func(command string, workingDir string) (int, error) {
		_, packageErr := os.Stat(path.Join(workingDir, outgoingPackage.PackageFileName))
		if packageErr != nil {
			return -1, packageErr
		}
		_, configErr := os.Stat(path.Join(workingDir, outgoingPackage.ConfigFileName))
		if configErr != nil {
			return -1, configErr
		}
		cachedFilesFound = true
		return 0, nil
	})
	action := action{outgoingPackage, false, packageregistry.RemoveForUpdate}
	err = actionPlan.executeHelper(packageRegistry, removeCommandHandler, packageregistry.CurrentPackageRegistry{}, &action, extensionEventManager)

	assert.NoError(t, err)
	assert.True(t, cachedFilesFound, "remove command should use the cached outgoing package")
	assert.EqualValues(t, 0, mhgCommunicator.DownloadPackageCount, "outgoing package should not be downloaded during upgrade")
	assert.EqualValues(t, 0, mhgCommunicator.DownloadConfigCount, "outgoing config should not be downloaded during upgrade")
	assert.Len(t, removeCommandHandler.Result, 1, "only the remove command should be executed")
	assert.EqualValues(t, outgoingPackage.RemoveCommand, removeCommandHandler.Result[0].command)
	_, err = os.Stat(outgoingPackage.DownloadDir)
	assert.True(t, os.IsNotExist(err), "downloadDir should be deleted")

	// ensure no checksum warning events are emitted
	eventFiles, err := os.ReadDir(handlerEnvironment.EventsFolder)
	assert.NoError(t, err)
	for _, eventFile := range eventFiles {
		eventJSON, readErr := os.ReadFile(path.Join(handlerEnvironment.EventsFolder, eventFile.Name()))
		assert.NoError(t, readErr)
		var event struct {
			EventLevel string `json:"EventLevel"`
		}
		assert.NoError(t, json.Unmarshal(eventJSON, &event))
		assert.NotEqual(t, "Warning", event.EventLevel, "successful checksum verification should not emit a warning event")
	}
}

func TestDeletedApp(t *testing.T) {
	initTest(t)
	defer cleanTest()
	action := action{vmAppPackageDeleted, false, packageregistry.Install}
	err := actionPlan.executeHelper(packageRegistry, commandHandler, packageregistry.CurrentPackageRegistry{}, &action, extensionEventManager)
	assert.Error(t, err, "The application test app, version 1.0.0 has been removed from the repository and cannot be installed. Please install a newer version of the application.")
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
