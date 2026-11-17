// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package customactionplan

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/exithelper"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/pkg/errors"
)

func (actionPlan *ActionPlan) executeHelper(
	commandHandler commandhandler.ICommandHandlerWithEnvVariables, registry ActionPackageRegistry,
	act *action, eem *extensionevents.ExtensionEventManager) (errorMessageToReturn error) {
	errorMessageToReturn = nil
	appName := act.vmAppPackage.ApplicationName
	version := act.vmAppPackage.Version
	tickCountFilePath := path.Join(actionPlan.environment.ConfigFolder, tickCountFile)

	if _, err := os.Stat(tickCountFilePath); os.IsNotExist(err) {
		tickCountFile, err := os.Create(tickCountFilePath)
		if err != nil {
			eem.LogErrorEvent("CustomActionTickCountFileCreated", "could not create tick count file")
			return err
		}
		eem.LogInformationalEvent("CustomActionTickCountFileCreated", "created tick count file")
		tickCountFile.Close()
	}

	// record new operation in the packageRegistry
	currAction := CustomActionPackage{}
	vmAppPackageCurrent := act.vmAppPackage
	registry[appName] = append(registry[appName], &currAction)
	currAction.ApplicationName = appName
	currAction.Version = version
	commandToExecute := act.Action.ActionScript
	commandParameters := act.Action.Parameters
	commandName := act.Action.ActionName
	currAction.ActionName = commandName
	currAction.Timestamp = act.Action.Timestamp
	currAction.Parameters = act.Action.Parameters
	currAction.Status = "In Progress"
	eem.LogInformationalEvent(
		"CustomActionStarted",
		fmt.Sprintf("Starting custom action cmd=%v, application=%v, version=%v, parameters=%v", commandName, appName, version, extdeserialization.GetParameterNames(act.Action)))

	// try to execute only if you have a valid command to execute
	if errorMessageToReturn == nil {
		// handle termination signals to handle reboot
		type ExecutionResult struct {
			retCode int
			err     error
		}
		completionSignal := make(chan ExecutionResult, 1)
		interruptSignal := make(chan os.Signal, 1)
		signal.Notify(interruptSignal, syscall.SIGTERM, syscall.SIGINT)

		params := make(map[string]string)
		for _, actionparams := range commandParameters {
			params[actionparams.ParameterName] = actionparams.ParameterValue
		}

		go func() {
			rCode, err := commandHandler.ExecuteWithEnvVariables(commandToExecute, vmAppPackageCurrent.DownloadDir, vmAppPackageCurrent.DownloadDir, true, actionPlan.logger, &params)
			completionSignal <- ExecutionResult{retCode: rCode, err: err}
			close(completionSignal)
		}()

		select {
		case compSignal := <-completionSignal:
			if compSignal.err != nil {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(compSignal.err, "Error executing custom action '%v': '%v'", commandName, commandToExecute))
			} else if compSignal.retCode != 0 {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Errorf("Custom action '%v' exited with non-zero error code: %v", commandName, commandToExecute))
			}
		case <-interruptSignal:
			// the command that we executed resulted in system reboot handle system reboot
			actionPlan.logger.Info("received terminate signal, system reboot detected")
			eem.LogInformationalEvent("System reboot detected",
				fmt.Sprintf("Custom action cmd=%v, application=%v, version=%v, parameters=%v, result=SUCCESS",
					commandName, appName, extdeserialization.GetParameterNames(act.Action), version))
			vmAppPackageCurrent.Result = fmt.Sprintf("reboot detected during custom action %v, marking operation as success", commandName)
			exithelper.Exiter.Exit(0)
		}
		signal.Stop(interruptSignal)
	}

	if errorMessageToReturn != nil {
		vmAppPackageCurrent.Result = fmt.Sprintf("%s %s %s", commandName, actionplan.Failed, errorMessageToReturn.Error())
	} else {
		vmAppPackageCurrent.Result = fmt.Sprintf("%s %s", commandName, actionplan.Success)
	}

	bytes, err := json.Marshal(act.Action.TickCount)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(tickCountFilePath, bytes, constants.FilePermissions_UserOnly_ReadWrite)
	if err != nil {
		currAction.Status = actionplan.Failed
		return markCommandFailed(commandName, appName, version, err, act, eem)
	}

	if errorMessageToReturn == nil {
		currAction.Status = actionplan.Success
		eem.LogInformationalEvent(
			"CustomActionCompleted",
			fmt.Sprintf("Completed custom action cmd=%v, application=%v, version=%v, parameters=%v, result=SUCCESS", commandName, appName, version, extdeserialization.GetParameterNames(act.Action)))
		return
	}
	currAction.Status = actionplan.Failed
	return markCommandFailed(commandName, appName, version, errorMessageToReturn, act, eem)
}

// RetryActionName is the reserved custom action name that reinstalls an application instead of running a script.
// Only this exact name (ignoring case) is reserved, so author-defined actions such as "retry" still run as scripts.
const RetryActionName = "Microsoft.Retry"

func isRetryAction(act *action) bool {
	return strings.EqualFold(act.Action.ActionName, RetryActionName)
}

// executeRetry reinstalls the application through the main action plan. The tick count is written before the
// reinstall starts so that a reboot during the reinstall does not cause the retry to run again.
// The remove and install results are appended to result. The returned ExecuteError is nil when the reinstall did
// not start; otherwise it carries the reinstall's errors, including deployment failures, for the caller to merge.
func (actionPlan *ActionPlan) executeRetry(
	commandHandler commandhandler.ICommandHandlerWithEnvVariables, registry ActionPackageRegistry,
	act *action, eem *extensionevents.ExtensionEventManager,
	result *actionplan.PackageOperationResults) (*actionplan.ExecuteError, error) {

	appName := act.vmAppPackage.ApplicationName
	version := act.vmAppPackage.Version
	commandName := act.Action.ActionName

	currAction := CustomActionPackage{
		ApplicationName: appName,
		Version:         version,
		ActionName:      commandName,
		Timestamp:       act.Action.Timestamp,
		Parameters:      act.Action.Parameters,
		Status:          "In Progress",
	}
	registry[appName] = append(registry[appName], &currAction)
	eem.LogInformationalEvent(
		"CustomActionStarted",
		fmt.Sprintf("Starting custom action cmd=%v, application=%v, version=%v", commandName, appName, version))

	fail := func(err error) (*actionplan.ExecuteError, error) {
		currAction.Status = actionplan.Failed
		return nil, markCommandFailed(commandName, appName, version, err, act, eem)
	}

	if actionPlan.mainActionPlan == nil || actionPlan.packageRegistryHandler == nil {
		return fail(errors.Errorf("Custom action '%v' cannot reinstall application %v because the reinstall dependencies were not provided", commandName, appName))
	}
	reinstallCommandHandler, ok := commandHandler.(commandhandler.ICommandHandler)
	if !ok {
		return fail(errors.Errorf("Custom action '%v' cannot reinstall application %v because the command handler cannot run install commands", commandName, appName))
	}

	if err := actionPlan.writeTickCount(act.Action.TickCount); err != nil {
		return fail(err)
	}

	reinstallError, reinstallResults := actionPlan.mainActionPlan.Reinstall(
		appName, actionPlan.incomingPackageCollection, actionPlan.packageRegistryHandler, eem, reinstallCommandHandler)
	*result = append(*result, *reinstallResults...)

	if err := reinstallError.GetCombinedExecuteError(); err != nil {
		currAction.Status = actionplan.Failed
		return reinstallError, markCommandFailed(commandName, appName, version, err, act, eem)
	}

	currAction.Status = actionplan.Success
	eem.LogInformationalEvent(
		"CustomActionCompleted",
		fmt.Sprintf("Completed custom action cmd=%v, application=%v, version=%v, result=SUCCESS", commandName, appName, version))
	return reinstallError, nil
}

func (actionPlan *ActionPlan) writeTickCount(tickCount uint64) error {
	bytes, err := json.Marshal(tickCount)
	if err != nil {
		return err
	}
	tickCountFilePath := path.Join(actionPlan.environment.ConfigFolder, tickCountFile)
	return ioutil.WriteFile(tickCountFilePath, bytes, constants.FilePermissions_UserOnly_ReadWrite)
}

func markCommandFailed(commandToExecute string, appName string, version string, err error, act *action, eem *extensionevents.ExtensionEventManager) error {
	eem.LogInformationalEvent(
		"CustomActionCompleted",
		fmt.Sprintf(
			"Completed custom action cmd=%v, application=%v, version=%v, parameters=%v, result=FAILED, reason=%v",
			commandToExecute, appName, version, extdeserialization.GetParameterNames(act.Action), err.Error()))

	return err
}
