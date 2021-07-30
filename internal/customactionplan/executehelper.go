package customactionplan

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/exithelper"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"syscall"
)

func (actionPlan *ActionPlan) executeHelper(
	commandHandler commandhandler.ICommandHandlerWithEnvVariables, registry ActionPackageRegistry,
	act *action, eem *extensionevents.ExtensionEventManager) (errorMessageToReturn error) {
	errorMessageToReturn = nil
	appName := act.vmAppPackage.ApplicationName
	version := act.vmAppPackage.Version

	tickCountFile := path.Join(actionPlan.environment.ConfigFolder, "tickCount")

	if _, err := os.Stat(tickCountFile); os.IsNotExist(err) {
		_, err = os.Create(tickCountFile)
		if err != nil {
			eem.LogErrorEvent("CustomActionTickCountFileCreated", "could not create tick count file")
			return err
		}
		eem.LogInformationalEvent("CustomActionTickCountFileCreated", "created tick count file")
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
		fmt.Sprintf("Starting custom action cmd=%v, application=%v, version=%v, parameters=%v", commandName, appName, version, getParameterNames(act.Action)))

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
		for _,actionparams := range commandParameters {
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
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(compSignal.err, "Error executing custom action %v: %v", commandName, commandToExecute))
			} else if compSignal.retCode != 0 {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Errorf("Custom action %v exited with non-zero error code: %v", commandName, commandToExecute))
			}
		case <-interruptSignal:
			// the command that we executed resulted in system reboot handle system reboot
			actionPlan.logger.Info("received terminate signal, system reboot detected")
			eem.LogInformationalEvent("System reboot detected",
				fmt.Sprintf("Custom action cmd=%v, application=%v, version=%v, parameters=%v, result=SUCCESS",
					commandName, appName, getParameterNames(act.Action), version))
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

	err = ioutil.WriteFile(tickCountFile, bytes, constants.FilePermissions_UserOnly_ReadWrite)
	if err != nil {
		currAction.Status = actionplan.Failed
		return markCommandFailed(commandName, appName, version, err, act, eem)
	}

	if errorMessageToReturn == nil {
		currAction.Status = actionplan.Success
		eem.LogInformationalEvent(
			"CustomActionCompleted",
			fmt.Sprintf("Completed custom action cmd=%v, application=%v, version=%v, parameters=%v, result=SUCCESS", commandName, appName, version, getParameterNames(act.Action)))
		return
	}
	currAction.Status = actionplan.Failed
	return markCommandFailed(commandName, appName, version, errorMessageToReturn, act, eem)
}

func markCommandFailed(commandToExecute string, appName string, version string, err error, act *action, eem *extensionevents.ExtensionEventManager) error {
	eem.LogInformationalEvent(
		"CustomActionCompleted",
		fmt.Sprintf(
			"Completed custom action cmd=%v, application=%v, version=%v, parameters=%v, result=FAILED, reason=%v",
			commandToExecute, appName, version, getParameterNames(act.Action), err.Error()))

	return err
}
