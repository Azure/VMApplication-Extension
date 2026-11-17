// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package customactionplan

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"

	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)

const (
	tickCountFile = "tickCount"
)

type action struct {
	// vmAppPackage is not a pointer type on purpose, we don't want it to be mutated
	vmAppPackage packageregistry.VMAppPackageCurrent
	Action       extdeserialization.ActionSetting
}

type ActionCollection []*action

func (a ActionCollection) Len() int {
	return len(a)
}

func (a ActionCollection) Less(i, j int) bool {
	return a[i].Action.TickCount < a[j].Action.TickCount
}

func (a ActionCollection) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

type ActionPlan struct {
	environment *handlerenv.HandlerEnvironment
	sortedOrder ActionCollection
	logger      *logging.ExtensionLogger
	// used by the reserved Microsoft.Retry action to reinstall an application
	mainActionPlan            *actionplan.ActionPlan
	packageRegistryHandler    packageregistry.IPackageRegistry
	incomingPackageCollection packageregistry.VMAppPackageIncomingCollection
}

type StatusErrorMessage string

func (err StatusErrorMessage) ToJsonString() string {
	return string(err)
}

func appendExecutionResult(executionResult *actionplan.PackageOperationResults, act *action, err error) {
	if err == nil {
		*executionResult = append(*executionResult, actionplan.PackageOperationResult{
			PackageName: act.vmAppPackage.ApplicationName,
			AppVersion:  act.vmAppPackage.Version,
			Operation:   act.Action.ActionName,
			Timestamp:   act.Action.Timestamp,
			Result:      actionplan.Success,
		})
	} else {
		*executionResult = append(*executionResult, actionplan.PackageOperationResult{
			PackageName: act.vmAppPackage.ApplicationName,
			AppVersion:  act.vmAppPackage.Version,
			Operation:   act.Action.ActionName,
			Timestamp:   act.Action.Timestamp,
			Result:      err.Error(),
		})
	}
}

func New(
	settings []*extdeserialization.VmAppSetting,
	appPackage packageregistry.CurrentPackageRegistry,
	environment *handlerenv.HandlerEnvironment,
	logger *logging.ExtensionLogger,
	mainActionPlan *actionplan.ActionPlan,
	packageRegistryHandler packageregistry.IPackageRegistry,
	incomingPackageCollection packageregistry.VMAppPackageIncomingCollection) (*ActionPlan, error) {

	actionPlan := ActionPlan{
		environment:               environment,
		sortedOrder:               make([]*action, 0),
		logger:                    logger,
		mainActionPlan:            mainActionPlan,
		packageRegistryHandler:    packageRegistryHandler,
		incomingPackageCollection: incomingPackageCollection,
	}

	tc := uint64(0)
	tickCountFile := path.Join(actionPlan.environment.ConfigFolder, tickCountFile)
	_, err := os.Stat(tickCountFile)
	if err != nil {
		logger.Info("Tick count file not found, setting tick count to 0. All actions will be executed.")
		tc = 0
	} else {
		tickCount, err := ioutil.ReadFile(tickCountFile)
		if err != nil {
			logger.Error("Cannot read tick count file")
		}
		tc, err = strconv.ParseUint(string(tickCount), 10, 64)
		if err != nil {
			tc = 0
			logger.Error("Tick count from file cannot be converted to integer")
		}
		logger.Info("Tick count file read, tick count: %v", tc)
	}

	for _, app := range settings {
		_, containsApp := appPackage[app.ApplicationName]
		if app.Actions != nil && len(app.Actions) != 0 && containsApp {
			for _, a := range app.Actions {
				if tc < a.TickCount {
					logger.Info("adding custom action %v to custom action plan", a.ActionName)
					newAction := action{
						vmAppPackage: *appPackage[app.ApplicationName],
						Action:       *a,
					}
					actionPlan.sortedOrder = append(actionPlan.sortedOrder, &newAction)
				} else {
					logger.Info("custom action %v has a low tick count (%v) compared to %v and will not be executed", a.ActionName, a.TickCount, tc)
				}
			}
		} else {
			logger.Info("application %v is not currently on the VM and cannot have its custom actions executed", app.ApplicationName)
		}
	}
	sort.Sort(actionPlan.sortedOrder)
	return &actionPlan, nil
}

func (actionPlan *ActionPlan) Execute(extEventManager *extensionevents.ExtensionEventManager, commandHandler commandhandler.ICommandHandlerWithEnvVariables, result *actionplan.PackageOperationResults) (*actionplan.ExecuteError, actionplan.IResult) {
	var executeError *actionplan.ExecuteError = &actionplan.ExecuteError{}
	executeError.SetFailedDeploymentErr(nil)
	executeError.SetCombinedExecuteErrors(nil)
	actionRegistry := GetCurrentCustomActions(actionPlan)

	for _, act := range actionPlan.sortedOrder {
		var newError error
		if isRetryAction(act) {
			var reinstallError *actionplan.ExecuteError
			reinstallError, newError = actionPlan.executeRetry(commandHandler, *actionRegistry, act, extEventManager, result)
			if reinstallError != nil {
				// carries the reinstall's errors, including deployment failures, so newError is not added again
				executeError.Merge(reinstallError)
			} else {
				executeError.SetCombinedExecuteErrors(extensionerrors.CombineErrors(executeError.GetCombinedExecuteError(), newError))
			}
		} else {
			newError = actionPlan.executeHelper(commandHandler, *actionRegistry, act, extEventManager)
			executeError.SetCombinedExecuteErrors(extensionerrors.CombineErrors(executeError.GetCombinedExecuteError(), newError))
		}
		// record the result before stopping so a failed custom action is visible in the status
		appendExecutionResult(result, act, newError)
		if newError != nil { //errors w/ tickcountfile or CA exits non-zero
			extEventManager.LogWarningEvent("CustomActionExecution", fmt.Sprintf("Custom Action %s failed, remaining actions will not be executed", act.Action.ActionName))
			break
		}
	}

	if combinedErrors := executeError.GetCombinedExecuteError(); combinedErrors != nil {
		extEventManager.LogWarningEvent("CustomActionExecution", combinedErrors.Error())
	}

	return executeError, result
}
