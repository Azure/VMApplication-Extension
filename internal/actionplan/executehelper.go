package actionplan

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/exithelper"
	"github.com/Azure/azure-extension-platform/pkg/extensionerrors"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/pkg/errors"
)

const MaxReboots = 3

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

	// prefix the app name and version for all extension events
	prefix := fmt.Sprintf("(%v:%v) ", appName, version)
	eem.SetPrefix(prefix)

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
	case packageregistry.RemoveForUpdate:
		commandToExecute = vmAppPackageCurrent.RemoveCommand
	case packageregistry.Remove:
		isDeleteOperation = true
		commandToExecute = vmAppPackageCurrent.RemoveCommand
	case packageregistry.Update:
		commandToExecute = vmAppPackageCurrent.UpdateCommand
	default:
		errorMessageToReturn = errors.Errorf("Unexpected Action to perform encountered %v", act.actionToPerform)
	}

	if vmAppPackageCurrent.NumRebootsOccurred == MaxReboots {
		actionPlan.logger.Error("The %v operation on application %v has resulted in %v reboots. Setting it to failed.", act.actionToPerform.ToString(), appName, MaxReboots)
		// Report failed status for application, reset reboot count in registry
		errorMessageToReturn = errors.Errorf("The %v operation on application %v has resulted in %v reboots. Cannot complete command.", act.actionToPerform.ToString(), appName, MaxReboots)
		vmAppPackageCurrent.NumRebootsOccurred = 0
	}

	actionPlan.logger.Info("Calling command '%v' for application %v, version %v", commandToExecute, appName, version)
	eem.LogInformationalEvent(
		"CommandStarted",
		fmt.Sprintf("Starting cmd=%v, application=%v, version=%v", commandToExecute, appName, version))

	// try to execute only if you have a valid command to execute
	if errorMessageToReturn == nil {
		if !isDeleteOperation {
			if vmAppPackageCurrent.IsDeleted {
				// application is marked as deleted. Provide a friendly error message to the customer
				actionPlan.logger.Error("The application %v, version %v has been deleted in the repository", appName, version)
				errorMessageToReturn = errors.Errorf(
					"The application %v, version %v has been removed from the repository and cannot be installed. Please install a newer version of the application.",
					appName, version)
			} else {
				// application is not marked as deleted
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
			}
		} else {
			// this is a delete operation, refrain from downloading anything just load existing packages
			// verify checksum
			packageFilePath := path.Join(vmAppPackageCurrent.DownloadDir, vmAppPackageCurrent.PackageFileName)
			downloadPath := vmAppPackageCurrent.GetWorkingDirectory(actionPlan.environment)
			vmAppPackageCurrent.DownloadDir = downloadPath
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
			start := time.Now().UnixNano() / int64(time.Millisecond)
			rCode, err := commandHandler.Execute(commandToExecute, vmAppPackageCurrent.DownloadDir, vmAppPackageCurrent.DownloadDir, true, actionPlan.logger)
			end := time.Now().UnixNano() / int64(time.Millisecond)
			executionInMs := end - start
			actionPlan.logger.Info("Command completed in %v ms", executionInMs)

			if err != nil {
				actionPlan.logger.Info(fmt.Sprintf("Command failed in directory '%v'. Details: %v", vmAppPackageCurrent.DownloadDir, err))
				eem.LogInformationalEvent("Command failed",
					fmt.Sprintf("cmd=%v, application=%v, version=%v, details=%v",
						commandToExecute, appName, version, err))
			}
			completionSignal <- ExecutionResult{retCode: rCode, err: err}
			close(completionSignal)
		}()

		select {
		case compSignal := <-completionSignal:
			if compSignal.err != nil {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(compSignal.err, "Error executing command '%v'", commandToExecute))
			} else if compSignal.retCode != 0 {
				errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Errorf("Command '%v' exited with non-zero error code", commandToExecute))
			}
			// Reset count if command successfully completed
			vmAppPackageCurrent.NumRebootsOccurred = 0
		case <-interruptSignal:
			// the command that we executed resulted in system reboot handle system reboot
			eem.LogInformationalEvent("System reboot detected",
				fmt.Sprintf("cmd=%v, application=%v, version=%v, result=Success",
					commandToExecute, appName, version))

			actionPlan.logger.Info("Received terminate signal, system reboot detected.")
			if vmAppPackageCurrent.RebootBehavior == packageregistry.Rerun {
				// vmPackageCurrent.OngoingOperation should remain the same (Install, Update, RemoveForUpdate, or Remove)
				// Increment reboot count
				vmAppPackageCurrent.NumRebootsOccurred += 1
				vmAppPackageCurrent.Result = fmt.Sprintf("Reboot detected during '%s' operation. Rerun operation after reboot.", vmAppPackageCurrent.OngoingOperation.ToString())
				actionPlan.logger.Info("Rerun operation '%v' after reboot. Number of reboots for operation so far: '%v'",
					vmAppPackageCurrent.OngoingOperation.ToString(), vmAppPackageCurrent.NumRebootsOccurred)
			} else {
				vmAppPackageCurrent.Result = fmt.Sprintf("Reboot detected during '%s' operation. No further action taken.", vmAppPackageCurrent.OngoingOperation.ToString())
				// Mark no additional action needs to be taken
				vmAppPackageCurrent.OngoingOperation = packageregistry.NoAction

				if vmAppPackageCurrent.OngoingOperation == packageregistry.RemoveForUpdate || vmAppPackageCurrent.OngoingOperation == packageregistry.Remove {
					delete(registry, appName)
					os.RemoveAll(vmAppPackageCurrent.DownloadDir)
				}

				actionPlan.logger.Info("Will not rerun operation '%v'. No further action will be taken.", vmAppPackageCurrent.OngoingOperation.ToString())
			}

			registryHandler.WriteToDisk(registry)
			exithelper.Exiter.Exit(0)
		}
		signal.Stop(interruptSignal)
	}

	if vmAppPackageCurrent.EnableApplicationEvents {
		logApplicationEvents(vmAppPackageCurrent.DownloadDir, appName, errorMessageToReturn, eem, actionPlan)
	}

	if isDeleteOperation {
		delete(registry, appName)
		// also cleanup directory
		deleteErr := os.RemoveAll(vmAppPackageCurrent.DownloadDir)
		errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, deleteErr)
	}

	if errorMessageToReturn != nil {
		vmAppPackageCurrent.Result = fmt.Sprintf("%s %s %s", vmAppPackageCurrent.OngoingOperation.ToString(), Failed, errorMessageToReturn.Error())
		vmAppPackageCurrent.OngoingOperation = packageregistry.Failed
	} else {
		vmAppPackageCurrent.Result = fmt.Sprintf("%s %s", vmAppPackageCurrent.OngoingOperation.ToString(), Success)
		vmAppPackageCurrent.OngoingOperation = packageregistry.NoAction
	}

	err = registryHandler.WriteToDisk(registry)
	if err != nil {
		return markCommandFailed(act.actionToPerform, commandToExecute, appName, version, err, eem)
	}

	if errorMessageToReturn == nil {
		eem.LogInformationalEvent(
			"CommandCompleted",
			fmt.Sprintf("Completed operation=%s, cmd=%v, application=%v, version=%v, result=Success", act.actionToPerform.ToString(), commandToExecute, appName, version))
		return
	}

	return markCommandFailed(act.actionToPerform, commandToExecute, appName, version, errorMessageToReturn, eem)
}

func markCommandFailed(actionPerformed packageregistry.ActionEnum, commandToExecute string, appName string, version string, err error, eem *extensionevents.ExtensionEventManager) error {
	eem.LogInformationalEvent(
		"CommandCompleted",
		fmt.Sprintf(
			"Completed operation=%s, cmd=%v, application=%v, version=%v, result=Failed, reason=%v",
			actionPerformed.ToString(), commandToExecute, appName, version, err.Error()))

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

func logApplicationEvents(downloadDir string, appName string, errorMessageToReturn error, eem *extensionevents.ExtensionEventManager, actionPlan *ActionPlan) () {
	//kusto log limit 
	maxEvents := 30
	eventCount := 0

	//read std err file to write to kusto
	stderrFileName := filepath.Join(downloadDir, "stderr")
	stderrFile, err := os.Open(stderrFileName)
    if err != nil {
		errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(err, "Error opening std err file for application %", appName))
    } else {
		// create reader + buffer
		stderrReader := bufio.NewReader(stderrFile)
    	stderrBuffer := make([]byte, 16*1024) //16 KB buffer due to event size constraint 
 
    	for eventCount <= maxEvents {
			// read content to buffer
			bytesRead, err := stderrReader.Read(stderrBuffer)
			if err != nil {
				if err != io.EOF {
					errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(err, "Error reading std err file for application %", appName))
				}
				break
        	}
			eem.LogErrorEvent(appName, string(stderrBuffer[:bytesRead]))
			eventCount++; 
    	}
	}
    defer stderrFile.Close()
 
	//read std out file to write to kusto
	stdoutFileName := filepath.Join(downloadDir, "stdout")
	stdoutFile, err := os.Open(stdoutFileName)
	if err != nil {
		errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(err, "Error opening std out file for application %", appName))
	} else {
		// create reader + buffer
		stdoutReader := bufio.NewReader(stdoutFile)
		stdoutBuffer := make([]byte, 16*1024) //16 KB buffer due to event size constraint

		for eventCount <= maxEvents {
			// read content to buffer
			bytesRead, err := stdoutReader.Read(stdoutBuffer)
			if err != nil {
				if err != io.EOF {
					errorMessageToReturn = extensionerrors.CombineErrors(errorMessageToReturn, errors.Wrapf(err, "Error reading std out file for application %", appName))
				}
				break
			}
			eem.LogInformationalEvent(appName, string(stdoutBuffer[:bytesRead]))
			eventCount++; 
		}
	}
	defer stdoutFile.Close()

	//check if max count reached and log summary 
	if (eventCount == maxEvents) {
		actionPlan.logger.Info("Maximum event count %d reached for application %s, view remaining logs in VM.", maxEvents, appName)
	} else {
		actionPlan.logger.Info("Logged all application events for application %s", appName)
	}
}
