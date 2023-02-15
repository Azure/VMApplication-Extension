package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/constants"
	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/lockedfile"
	"github.com/Azure/azure-extension-platform/pkg/seqno"
	"github.com/Azure/azure-extension-platform/pkg/status"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

var (
	ExtensionVersion      = "1.0.10" // should be assigned at compile time, do not edit in code
	reportStatusFunc      = utils.ReportStatus
	getVMExtensionFunc    = getVMExtension
	setSequenceNumberFunc = seqno.SetSequenceNumber
)

const (
	vmPackagesSetting       = "vmPackages"
	operationInstall        = "install"
	operationUpdate         = "update"
	operationRemove         = "remove"
	filelockTimeoutDuration = 15 * time.Minute
)

func main() {
	err := getExtensionAndRun(os.Args)
	if err != nil {
		os.Exit(2)
	}
}

func getExtensionAndRun(arguments []string) error {
	// require SeqNoChange is set to false because we want the extension to ensure that the packages are in sync with the desired packages
	ext, err := getVMExtensionFunc()
	if err != nil {
		return err
	}

	if len(arguments) != 2 {
		ext.ExtensionLogger.Error("ExtensionError", "vm-application-manager requires an argument")
		ext.ExtensionEvents.LogCriticalEvent("ExtensionError", "vm-application-manager requires an argument")
		return errors.Errorf("vm-application-manager requires an argument")
	}
	command := arguments[1]
	if command == vmextensionhelper.EnableOperation.ToString() {
		// do not call ext.Do() handle the enable command in the extension
		requestedSequenceNumber, err := ext.GetRequestedSequenceNumber()
		if err != nil {
			msg := "could not determine requested sequence number"
			ext.ExtensionLogger.Error("%s: %v", msg, err)
			ext.ExtensionEvents.LogCriticalEvent("ExtensionError", fmt.Sprintf("%s: %v", msg, err))
			return err
		}
		hostgaCommunicator := hostgacommunicator.HostGaCommunicator{}
		enableError := customEnable(ext, &hostgaCommunicator, requestedSequenceNumber)

		if enableError != nil {
			// write failure status
			ext.ExtensionLogger.Error(enableError.Error())
			ext.ExtensionEvents.LogErrorEvent("Enable Failed", enableError.Error())
			_, ok := enableError.(*utils.StatusSaveError)
			if !ok {
				// this means we had an error other than trying to save status file
				statusMessage := enableError.Error()
				err := reportStatusFunc(ext.HandlerEnv, requestedSequenceNumber, status.StatusError, vmextensionhelper.EnableOperation.ToStatusName(), statusMessage)
				if err != nil {
					errorMessage := fmt.Sprintf("failed to save status file: %s", err.Error())
					ext.ExtensionLogger.Error(errorMessage)
					ext.ExtensionEvents.LogErrorEvent("Save Status", errorMessage)
					return err
				}
			}
		}
	} else {
		ext.Do()
	}

	return nil
}

func dummyVMAppEnableCallback(ext *vmextensionhelper.VMExtension) (string, error) {
	return "", nil
}

func getVMExtension() (*vmextensionhelper.VMExtension, error) {
	ii, err := vmextensionhelper.GetInitializationInfo(constants.ExtensionName, ExtensionVersion, false, dummyVMAppEnableCallback)
	if err != nil {
		return nil, err
	}

	ii.UninstallCallback = vmAppUninstallCallback
	ii.UpdateCallback = vmAppUpdateCallback
	ii.LogFileNamePattern = "VmAppExt_%v.log"

	ext, err := vmextensionhelper.GetVMExtension(ii)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

// Perform VMApp operations and write status
// If returned error is not nil, status file hasn't been written
func customEnable(ext *vmextensionhelper.VMExtension, hostgaCommunicator hostgacommunicator.IHostGaCommunicator, requestedSequenceNumber uint) error {

	ext.ExtensionEvents.LogInformationalEvent("Starting", fmt.Sprintf("VmApplications extension starting, PID %d", os.Getpid()))

	// try to get file lock by accessing package registry
	// this section is to ensure that only once instance of the VMAppExtension runs at any given time
	packageRegistry, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		// log error and exit
		switch err.(type) {
		case *lockedfile.FileLockTimeoutError:
			ext.ExtensionEvents.LogErrorEvent(
				"Acquire lock",
				fmt.Sprintf("Failed to acquire package registry lock. Request timed out. It is likely that another instance is already running %v", err.Error()))
		default:
			ext.ExtensionEvents.LogErrorEvent(
				"Acquire lock",
				fmt.Sprintf("Failed to acquire package registry lock. %v", err.Error()))
		}
		ext.ExtensionEvents.LogInformationalEvent("Exiting", fmt.Sprintf("VmApplications extension exiting, PID %d", os.Getpid()))
		return err
	}
	defer packageRegistry.Close()

	settings, err := ext.GetSettings()
	if err != nil {
		return errors.Wrap(err, "could not get extension settings")
	}

	protSettings, err := extdeserialization.GetVMAppProtectedSettings(settings)
	if err != nil {
		return errors.Wrap(err, "Could not deserialize protected settings")
	}
	vmAppIncomingCollection, err := getVMAppIncomingCollection(protSettings, hostgaCommunicator, ext.ExtensionLogger)
	if err != nil {
		return errors.Wrap(err, "resolving packages failed")
	}

	currentPackageRegistry, err := packageRegistry.GetExistingPackages()
	if err != nil {
		return errors.Wrap(err, "could not read current package registry")
	}

	commandHandler := commandhandler.CommandHandler{}

	actionPlan := actionplan.New(currentPackageRegistry, vmAppIncomingCollection, ext.HandlerEnv, hostgaCommunicator, ext.ExtensionLogger)
	executeError, actionplanResult := actionPlan.Execute(packageRegistry, ext.ExtensionEvents, &commandHandler)

	if executeError.GetErrorIfDeploymentFailed() != nil {
		ext.ExtensionEvents.LogErrorEvent(
			"Completed",
			fmt.Sprintf("VmApplications extension finished. Result=Failure;Reason=%v", executeError.GetErrorIfDeploymentFailed().Error()))
	} else {
		ext.ExtensionEvents.LogInformationalEvent("Completed", "VmApplications extension finished. Result=Success")
	}

	currentPackageRegistry, err = packageRegistry.GetExistingPackages()
	if err != nil {
		return errors.Wrapf(err, "could not get package registry")
	}

	// write success status if requested sequence number is newer
	if ext.CurrentSequenceNumber != nil {
		if requestedSequenceNumber > *ext.CurrentSequenceNumber {
			var statusResult status.StatusType
			statusMessage := getStatusMessage(currentPackageRegistry.GetPackageCollection(), executeError, actionplanResult)
			if executeError.GetErrorIfDeploymentFailed() == nil { // treatFailureAsDeploymentFailure
				statusResult = status.StatusSuccess
			} else {
				statusResult = status.StatusError
			}
			err := utils.ReportStatus(ext.HandlerEnv, requestedSequenceNumber, statusResult, vmextensionhelper.EnableOperation.ToStatusName(), statusMessage)
			if err != nil {
				errorMessage := fmt.Sprintf("failed to save status file: %s", err.Error())
				ext.ExtensionLogger.Error(errorMessage)
				ext.ExtensionEvents.LogErrorEvent("Save Status", errorMessage)
				return err
			}
			// update the sequence number that has been executed
			if err := setSequenceNumberFunc(constants.ExtensionName, ExtensionVersion, requestedSequenceNumber); err != nil {
				errorMessage := fmt.Sprintf("Failed to update sequence number to %d: %s", requestedSequenceNumber, err.Error())
				ext.ExtensionLogger.Error(errorMessage)
				ext.ExtensionEvents.LogErrorEvent("Update Sequence Number", errorMessage)
			}
		} else {
			message := fmt.Sprintf("Skipped updating status file. Requested sequence number %d, current sequence number %d.", requestedSequenceNumber, *ext.CurrentSequenceNumber)
			ext.ExtensionLogger.Info(message)
			ext.ExtensionEvents.LogInformationalEvent("Save Status", message)
		}
	}
	return nil
}

// Callback indicating the extension is being removed
func vmAppUninstallCallback(ext *vmextensionhelper.VMExtension) error {
	ext.ExtensionEvents.LogInformationalEvent("Uninstalling", "VmApplications extension - removing all applications for uninstall")
	hostGaCommunicator := hostgacommunicator.HostGaCommunicator{}
	err := doVmAppUninstallCallback(ext, &hostGaCommunicator)
	if err == nil {
		ext.ExtensionEvents.LogInformationalEvent("Completed", "VmApplications extension uninstalled. Result=Success")
	} else {
		ext.ExtensionEvents.LogInformationalEvent(
			"Completed",
			fmt.Sprintf("VmApplications extension uninstall finished. Result=Failure;Reason=%v", err.Error()))
	}
	return err
}

func doVmAppUninstallCallback(ext *vmextensionhelper.VMExtension, hostGaCommunicator hostgacommunicator.IHostGaCommunicator) error {
	packageRegistry, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		return errors.Wrapf(err, "could not create package registry")
	}
	defer packageRegistry.Close()

	currentPackageRegistry, err := packageRegistry.GetExistingPackages()
	if err != nil {
		return errors.Wrapf(err, "could not read current package registry")
	}

	// Create an empty incoming collection so we'll create an action plan to remove all applications
	emptyIncomingCollection := make(packageregistry.VMAppPackageIncomingCollection, 0)

	actionPlan := actionplan.New(currentPackageRegistry, emptyIncomingCollection, ext.HandlerEnv, hostGaCommunicator, ext.ExtensionLogger)
	commandHandler := commandhandler.CommandHandler{}

	// Removing applications is best effort, so even if there are errors here, we ignore them
	_, _ = actionPlan.Execute(packageRegistry, ext.ExtensionEvents, &commandHandler)

	return nil
}
