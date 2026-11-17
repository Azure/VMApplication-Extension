// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package customactionplan

import (
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/assert"
)

const retryTickCount uint64 = 10193113

var mockCommandFailOnDemand CommandExecutor = func(command string, workingDir string) (int, error) {
	if strings.HasPrefix(command, "fail") {
		return 1, nil
	}
	return 0, nil
}

func newRetryActionSetting(actionName string, script string, tickCount uint64) *extdeserialization.ActionSetting {
	return &extdeserialization.ActionSetting{
		ActionName:   actionName,
		ActionScript: script,
		Timestamp:    "20210604T155300Z",
		Parameters:   []extdeserialization.ActionParameter{},
		TickCount:    tickCount,
	}
}

func newRetryApps(installCommand string, treatFailureAsDeploymentFailure bool) (*packageregistry.VMAppPackageCurrent, packageregistry.VMAppPackageIncomingCollection) {
	existing := &packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "1.0",
		InstallCommand:   installCommand,
		RemoveCommand:    "remove app1",
		OngoingOperation: packageregistry.Failed,
		Result:           "Install FAILED",
	}
	incoming := packageregistry.VMAppPackageIncomingCollection{
		&packageregistry.VMAppPackageIncoming{
			ApplicationName:                 "app1",
			Version:                         "1.0",
			InstallCommand:                  installCommand,
			RemoveCommand:                   "remove app1",
			TreatFailureAsDeploymentFailure: treatFailureAsDeploymentFailure,
		},
	}
	return existing, incoming
}

func executeRetryActionPlan(t *testing.T,
	actions []*extdeserialization.ActionSetting,
	existing *packageregistry.VMAppPackageCurrent,
	incoming packageregistry.VMAppPackageIncomingCollection,
	cmdHandler *CommandHandlerMock,
	provideReinstallDependencies bool) (*actionplan.ExecuteError, actionplan.PackageOperationResults, packageregistry.CurrentPackageRegistry) {

	packageReg, err := packageregistry.New(extLogger, environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	assert.NoError(t, packageReg.WriteToDisk(packageregistry.CurrentPackageRegistry{existing.ApplicationName: existing}))
	current, err := packageReg.GetExistingPackages()
	assert.NoError(t, err)

	settings := []*extdeserialization.VmAppSetting{{ApplicationName: existing.ApplicationName, Order: &one, Actions: actions}}
	var customActionPlan *ActionPlan
	if provideReinstallDependencies {
		mainActionPlan := actionplan.New(current, incoming, environment, new(NoopHostGaComminucator), extLogger)
		customActionPlan, err = New(settings, current, environment, extLogger, mainActionPlan, packageReg, incoming)
	} else {
		customActionPlan, err = New(settings, current, environment, extLogger, nil, nil, nil)
	}
	assert.NoError(t, err)

	eem := extensionevents.New(logging.New(nil), getHandlerEnvironment())
	results := actionplan.PackageOperationResults{}
	executeError, _ := customActionPlan.Execute(eem, cmdHandler, &results)

	newReg, err := packageReg.GetExistingPackages()
	assert.NoError(t, err)
	return executeError, results, newReg
}

func TestRetryActionReinstallsApplication(t *testing.T) {
	cleanupTest()
	initializeTest(t)
	defer cleanupTest()
	existing, incoming := newRetryApps("install app1", false)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	actions := []*extdeserialization.ActionSetting{newRetryActionSetting("microsoft.RETRY", "", retryTickCount)}

	executeError, results, newReg := executeRetryActionPlan(t, actions, existing, incoming, cmdHandler, true)

	assert.Len(t, cmdHandler.Result, 2)
	assert.EqualValues(t, "remove app1", cmdHandler.Result[0].command)
	assert.EqualValues(t, "install app1", cmdHandler.Result[1].command)
	assert.Len(t, results, 3)
	assert.EqualValues(t, packageregistry.RemoveForUpdate.ToString(), results[0].Operation)
	assert.EqualValues(t, actionplan.Success, results[0].Result)
	assert.EqualValues(t, packageregistry.Install.ToString(), results[1].Operation)
	assert.EqualValues(t, actionplan.Success, results[1].Result)
	assert.EqualValues(t, "microsoft.RETRY", results[2].Operation)
	assert.EqualValues(t, actionplan.Success, results[2].Result)
	assert.Equal(t, packageregistry.NoAction, newReg["app1"].OngoingOperation)
	assert.Contains(t, newReg["app1"].Result, actionplan.Success)
	assertTickCountFileCorrect(t, retryTickCount)
	assert.NoError(t, executeError.GetCombinedExecuteError())
}

func TestActionNamedRetryRunsAsScript(t *testing.T) {
	cleanupTest()
	initializeTest(t)
	defer cleanupTest()
	existing, incoming := newRetryApps("install app1", false)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	actions := []*extdeserialization.ActionSetting{newRetryActionSetting("retry", "echo author retry", retryTickCount)}

	executeError, results, _ := executeRetryActionPlan(t, actions, existing, incoming, cmdHandler, true)

	assert.Len(t, cmdHandler.Result, 1, "only the author's script must run")
	assert.EqualValues(t, "echo author retry", cmdHandler.Result[0].command)
	assert.Len(t, results, 1)
	assert.EqualValues(t, "retry", results[0].Operation)
	assert.EqualValues(t, actionplan.Success, results[0].Result)
	assert.NoError(t, executeError.GetCombinedExecuteError())
}

func TestRetryInstallFailureFailsDeploymentWhenTreatFailureAsDeploymentFailure(t *testing.T) {
	cleanupTest()
	initializeTest(t)
	defer cleanupTest()
	existing, incoming := newRetryApps("fail install app1", true)
	cmdHandler := NewCommandHandlerMock(mockCommandFailOnDemand)
	actions := []*extdeserialization.ActionSetting{newRetryActionSetting(RetryActionName, "", retryTickCount)}

	executeError, results, newReg := executeRetryActionPlan(t, actions, existing, incoming, cmdHandler, true)

	assert.Len(t, results, 3)
	assert.EqualValues(t, actionplan.Success, results[0].Result, "remove must succeed")
	assert.EqualValues(t, packageregistry.Install.ToString(), results[1].Operation)
	assert.Contains(t, results[1].Result, "non-zero error code")
	assert.True(t, results[1].TreatFailureAsDeploymentFailure)
	assert.EqualValues(t, RetryActionName, results[2].Operation)
	assert.Contains(t, results[2].Result, "non-zero error code")
	assert.Equal(t, packageregistry.Failed, newReg["app1"].OngoingOperation)
	deploymentErr := executeError.GetErrorIfDeploymentFailed()
	assert.Error(t, deploymentErr)
	assert.Contains(t, deploymentErr.Error(), "app1")
	assertTickCountFileCorrect(t, retryTickCount)
}

func TestRetryWithoutReinstallDependenciesFails(t *testing.T) {
	cleanupTest()
	initializeTest(t)
	defer cleanupTest()
	existing, incoming := newRetryApps("install app1", true)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	actions := []*extdeserialization.ActionSetting{newRetryActionSetting(RetryActionName, "", retryTickCount)}

	executeError, results, _ := executeRetryActionPlan(t, actions, existing, incoming, cmdHandler, false)

	assert.Len(t, cmdHandler.Result, 0)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Result, "reinstall dependencies were not provided")
	assert.NoError(t, executeError.GetErrorIfDeploymentFailed(), "nothing was installed, so the deployment must not fail")
	assert.Error(t, executeError.GetCombinedExecuteError())
	_, err := os.Stat(path.Join(environment.ConfigFolder, tickCountFile))
	assert.True(t, os.IsNotExist(err), "the tick count must not be written when the reinstall does not start")
}

func TestRetryRunsInTickCountOrderWithOtherActions(t *testing.T) {
	cleanupTest()
	initializeTest(t)
	defer cleanupTest()
	existing, incoming := newRetryApps("install app1", false)
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	actions := []*extdeserialization.ActionSetting{
		newRetryActionSetting("action2", "echo after retry", retryTickCount+2),
		newRetryActionSetting(RetryActionName, "", retryTickCount+1),
	}

	executeError, results, _ := executeRetryActionPlan(t, actions, existing, incoming, cmdHandler, true)

	assert.Len(t, cmdHandler.Result, 3)
	assert.EqualValues(t, "remove app1", cmdHandler.Result[0].command)
	assert.EqualValues(t, "install app1", cmdHandler.Result[1].command)
	assert.EqualValues(t, "echo after retry", cmdHandler.Result[2].command)
	assert.Len(t, results, 4)
	assert.EqualValues(t, RetryActionName, results[2].Operation)
	assert.EqualValues(t, "action2", results[3].Operation)
	assertTickCountFileCorrect(t, retryTickCount+2)
	assert.NoError(t, executeError.GetCombinedExecuteError())
}
