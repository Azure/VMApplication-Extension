package actionplan

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/exithelper"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/pkg/errors"
)

func (actionPlan *ActionPlan) executeHelper(registryHandler packageregistry.IPackageRegistry,
	commandHandler commandhandler.ICommandHandler, registry packageregistry.CurrentPackageRegistry,
	act *action, eem *extensionevents.ExtensionEventManager) (errorMessageToReturn error) {
	errorMessageToReturn = nil
	appName := act.vmAppPackage.ApplicationName
	version := act.vmAppPackage.Version

	// record new operation in the packageRegistry
	vmAppPackageCurrent := act.vmAppPackage
	registry[appName] = &vmAppPackageCurrent
	vmAppPackageCurrent.OngoingOperation = act.actionToPerform

	// return early for Cleanup operation
	if vmAppPackageCurrent.OngoingOperation == packageregistry.Cleanup {
		delete(registry, appName)
		return registryHandler.WriteToDisk(registry)
	}

	err := registryHandler.WriteToDisk(registry)
	if err != nil {
		return err
	}

	var commandToExecute string
	var isDeleteOperation = false
	switch act.actionToPerform {
	case packageregistry.Install:
		commandToExecute = vmAppPackageCurrent.InstallCommand
	case packageregistry.Remove:
		isDeleteOperation = true
		commandToExecute = vmAppPackageCurrent.RemoveCommand
	case packageregistry.Update:
		commandToExecute = vmAppPackageCurrent.UpdateCommand
	default:
		errorMessageToReturn = errors.Errorf("Unexpected Action to perform encountered %v", act.actionToPerform)
	}

	actionPlan.logger.Info("Calling command %v for application %v, version %v", commandToExecute, appName, version)
	eem.LogInformationalEvent(
		"CommandStarted",
		fmt.Sprintf("Starting cmd=%v, application=%v, version=%v", commandToExecute, appName, version))

	// try to execute only if you have a valid command to execute

	if errorMessageToReturn == nil {
		if !isDeleteOperation {
			downloadPath := vmAppPackageCurrent.GetWorkingDirectory(actionPlan.environment)
			vmAppPackageCurrent.DownloadDir = downloadPath

			if err := os.MkdirAll(downloadPath, constants.FilePermissions_UserOnly_ReadWriteExecute); err != nil {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(err, "failed to create download directory %s", downloadPath))
			}
			// proceed only if there was no error in the previous operation
			if err == nil {
				// download packages
				downloadPackageFileName := path.Join(downloadPath, vmAppPackageCurrent.PackageFileName)
				if err := actionPlan.hostGaCommunicator.DownloadPackage(actionPlan.logger, vmAppPackageCurrent.ApplicationName, downloadPackageFileName); err != nil {
					actionPlan.logger.Error("Failed to download package for application %v, version %v. Error: %v", appName, version, err.Error())
					errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(err, "failed to download package file %s", downloadPackageFileName))
				}
				if err == nil {
					if packageFileChecksum, err := getMD5CheckSum(downloadPackageFileName); err == nil {
						vmAppPackageCurrent.PackageFileMD5Checksum = packageFileChecksum
					} else {
						eem.LogWarningEvent("calculate checksum", fmt.Sprintf("could not get checksum for file %s, error: %s", downloadPackageFileName, err.Error()))
					}
				}

				// download configuration
				if vmAppPackageCurrent.ConfigExists {
					downloadConfigFileName := path.Join(downloadPath, vmAppPackageCurrent.ConfigFileName)
					if err := actionPlan.hostGaCommunicator.DownloadConfig(actionPlan.logger, vmAppPackageCurrent.ApplicationName, downloadConfigFileName); err != nil {
						actionPlan.logger.Error("Failed to download config for application %v, version %v. Error: %v", appName, version, err.Error())
						errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(err, "failed to download config file %s", downloadConfigFileName))
					}
					if err == nil {
						if configFileChecksum, err := getMD5CheckSum(downloadConfigFileName); err == nil {
							vmAppPackageCurrent.ConfigFileMD5Checksum = configFileChecksum
						} else {
							eem.LogWarningEvent("calculate checksum", fmt.Sprintf("could not get checksum for file %s, error: %s", downloadConfigFileName, err.Error()))
						}
					}
				}
			}
		} else {
			// this is a delete operation, refrain from downloading anything just load existing packages
			// verify checksum
			packageFilePath := path.Join(vmAppPackageCurrent.DownloadDir, vmAppPackageCurrent.PackageFileName)
			if vmAppPackageCurrent.PackageFileMD5Checksum != nil {
				isMatch, err := verifyMD5CheckSum(packageFilePath, vmAppPackageCurrent.PackageFileMD5Checksum)
				if err != nil {
					eem.LogWarningEvent("verify checksum", fmt.Sprintf("could not get checksum for file %s, error: %s", packageFilePath, err.Error()))
				} else if !isMatch {
					eem.LogWarningEvent("verify checksum", fmt.Sprintf("the checksum for file %s does not match", packageFilePath))
				}
			}
			if vmAppPackageCurrent.ConfigExists && vmAppPackageCurrent.ConfigFileMD5Checksum != nil {
				configFilePath := path.Join(vmAppPackageCurrent.DownloadDir, vmAppPackageCurrent.ConfigFileName)
				isMatch, err := verifyMD5CheckSum(configFilePath, vmAppPackageCurrent.ConfigFileMD5Checksum)
				if err != nil {
					eem.LogWarningEvent("verify checksum", fmt.Sprintf("could not get checksum for file %s, error: %s", configFilePath, err.Error()))
				} else if !isMatch {
					eem.LogWarningEvent("verify checksum", fmt.Sprintf("the checksum for file %s does not match", configFilePath))
				}
			}
		}

		// handle termination signals to handle reboot
		type ExecutionResult struct {
			retCode int
			err     error
		}

		completionSignal := make(chan ExecutionResult, 1)
		interruptSignal := make(chan os.Signal, 1)
		signal.Notify(interruptSignal, syscall.SIGTERM, syscall.SIGINT)

		go func() {
			rCode, err := commandHandler.Execute(commandToExecute, vmAppPackageCurrent.DownloadDir, vmAppPackageCurrent.DownloadDir, true, actionPlan.logger)
			completionSignal <- ExecutionResult{retCode: rCode, err: err}
			close(completionSignal)
		}()

		select {
		case compSignal := <-completionSignal:
			if compSignal.err != nil {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(compSignal.err, "Error executing command %v", commandToExecute))
			} else if compSignal.retCode != 0 {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Errorf("Command %v exited with non-zero error code", commandToExecute))
			}
		case <-interruptSignal:
			// the command that we executed resulted in system reboot handle system reboot
			actionPlan.logger.Info("received terminate signal, system reboot detected")
			eem.LogInformationalEvent("System reboot detected",
				fmt.Sprintf("cmd=%v, application=%v, version=%v, result=Success",
					commandToExecute, appName, version))
			// depending on the action to perform, we either mark than no additional action needs to be taken, or in case of remove action, mark the app as removed
			switch act.actionToPerform {
			case packageregistry.Install:
				vmAppPackageCurrent.OngoingOperation = packageregistry.NoAction
				vmAppPackageCurrent.Result = "reboot detected during install, marking operation as success"
			case packageregistry.Update:
				vmAppPackageCurrent.OngoingOperation = packageregistry.NoAction
				vmAppPackageCurrent.Result = "reboot detected during update, marking operation as success"
			case packageregistry.Remove:
				delete(registry, appName)
				os.RemoveAll(vmAppPackageCurrent.DownloadDir)
			}
			registryHandler.WriteToDisk(registry)
			exithelper.Exiter.Exit(0)
		}
		signal.Stop(interruptSignal)
	}

	if errorMessageToReturn != nil {
		vmAppPackageCurrent.Result = fmt.Sprintf("%s %s %s", vmAppPackageCurrent.OngoingOperation.ToString(), Failed, errorMessageToReturn.Error())
		vmAppPackageCurrent.OngoingOperation = packageregistry.Failed
	} else {
		if isDeleteOperation {
			delete(registry, appName)
			// also cleanup directory
			deleteErr := os.RemoveAll(vmAppPackageCurrent.DownloadDir)
			errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, deleteErr)
		} else {
			vmAppPackageCurrent.Result = fmt.Sprintf("%s %s", vmAppPackageCurrent.OngoingOperation.ToString(), Success)
			vmAppPackageCurrent.OngoingOperation = packageregistry.NoAction
		}
	}
	err = registryHandler.WriteToDisk(registry)
	if err != nil {
		return markCommandFailed(commandToExecute, appName, version, err, eem)
	}

	if errorMessageToReturn == nil {
		eem.LogInformationalEvent(
			"CommandCompleted",
			fmt.Sprintf("Completed cmd=%v, application=%v, version=%v, result=Success", commandToExecute, appName, version))
		return
	}

	return markCommandFailed(commandToExecute, appName, version, errorMessageToReturn, eem)
}

func markCommandFailed(commandToExecute string, appName string, version string, err error, eem *extensionevents.ExtensionEventManager) error {
	eem.LogInformationalEvent(
		"CommandCompleted",
		fmt.Sprintf(
			"Completed cmd=%v, application=%v, version=%v, result=Failed, reason=%v",
			commandToExecute, appName, version, err.Error()))

	return err
}

func getMD5CheckSum(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	md5Hasher := md5.New()
	_, err = io.Copy(md5Hasher, file)
	if err != nil {
		return nil, err
	}
	return md5Hasher.Sum(nil), nil
}

func verifyMD5CheckSum(filePath string, checkSum []byte) (bool, error) {
	checkSumNew, err := getMD5CheckSum(filePath)
	if err != nil {
		return false, err
	}
	return bytes.Equal(checkSumNew, checkSum), nil
}
