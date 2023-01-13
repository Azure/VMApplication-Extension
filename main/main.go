package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/customactionplan"
	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/lockedfile"
	"github.com/Azure/azure-extension-platform/pkg/status"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

// Note: not const so test can change them
var (
	extensionVersion = "1.0.10"
)

const (
	vmPackagesSetting       = "vmPackages"
	operationInstall        = "install"
	operationUpdate         = "update"
	operationRemove         = "remove"
	filelockTimeoutDuration = 15 * time.Minute
)

func main() {
	err := getExtensionAndRun()
	if err != nil {
		os.Exit(2)
	}
}

func getExtensionAndRun() error {
	// require SeqNoChange is set to false because we want the extension to ensure that the packages are in sync with the desired packages
	ii, err := vmextensionhelper.GetInitializationInfo(extensionName, extensionVersion, false, dummyVMAppEnableCallback)
	if err != nil {
		return err
	}

	ii.UninstallCallback = vmAppUninstallCallback
	ii.UpdateCallback = vmAppUpdateCallback
	ii.LogFileNamePattern = "VmAppExt_%v.log"

	ext, err := vmextensionhelper.GetVMExtension(ii)
	if err != nil {
		return err
	}

	if len(os.Args) != 2 {
		ext.ExtensionEvents.LogCriticalEvent("ExtensionError", "vm-application-manager requires an argument")
		return errors.Errorf("vm-application-manager requires an argument")
	}
	command := os.Args[1]
	if command == vmextensionhelper.EnableOperation.ToString() {
		// do not call ext.Do() handle the enable command in the extension

	} else {
		ext.Do()
	}

	return nil
}

func dummyVMAppEnableCallback(ext *vmextensionhelper.VMExtension) (string, error) {
	return "", nil
}

// Callback indicating the operation is enable
func customEnable(ext *vmextensionhelper.VMExtension) error {

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

	// only write a transitioning status if the sequence number has increased
	requestedSequenceNumber, err := ext.GetRequestedSequenceNumber()
	if err != nil {
		msg := "could not determine requested sequence number"
		ext.ExtensionLogger.Error("%s: %v", msg, err)
		return err
	}

	if ext.CurrentSequenceNumber != nil && requestedSequenceNumber > *ext.CurrentSequenceNumber {
		err = utils.ReportStatus(ext, status.StatusTransitioning, vmextensionhelper.EnableOperation.ToStatusName(), "transitioning")
		if err != nil {
			return err
		}
	}

	hostGaCommunicator := hostgacommunicator.HostGaCommunicator{}
	settings, err := ext.GetSettings()
	if err != nil {
		return errors.Wrap(err, "could not get extension settings")
	}

	protSettings, err := extdeserialization.GetVMAppProtectedSettings(settings)
	if err != nil {
		return errors.Wrap(err, "Could not deserialize protected settings")
	}
	vmAppIncomingCollection, err := getVMAppIncomingCollection(protSettings, &hostGaCommunicator, ext.ExtensionLogger)
	if err != nil {
		return errors.Wrap(err, "resolving packages failed")
	}

	currentPackageRegistry, err := packageRegistry.GetExistingPackages()

	if err != nil {
		return errors.Wrap(err, "could not read current package registry")
	}

	actionplanResult, customActionResult, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	if err == nil {
		ext.ExtensionEvents.LogInformationalEvent("Completed", "VmApplications extension finished. Result=Success")

		actionPlanPackageOperationResult, ok := actionplanResult.(*actionplan.PackageOperationResults)
		currentPackageRegistry, err = packageRegistry.GetExistingPackages()
		if !ok {
			return getStatusMessage(currentPackageRegistry.GetPackageCollection(), actionplanResult), nil
		}
		if err != nil {
			return "could not get package registry", err
		}
	} else {
		ext.ExtensionEvents.LogErrorEvent(
			"Completed",
			fmt.Sprintf("VmApplications extension finished. Result=Failure;Reason=%v", err.Error()))
	}

	return result, err
}

func doVmAppEnableCallback(ext *vmextensionhelper.VMExtension, hostGaCommunicator hostgacommunicator.IHostGaCommunicator) (actionplan.IResult, actionplan.IResult, error) {

	actionPlan := actionplan.New(currentPackageRegistry, vmAppIncomingCollection, ext.HandlerEnv, hostGaCommunicator, ext.ExtensionLogger)
	commandHandler := commandhandler.CommandHandler{}

	// actionPlan.Execute can fail partially, but we mark the overall process as success
	// errors are sent in the status message
	executeError, result := actionPlan.Execute(packageRegistry, ext.ExtensionEvents, &commandHandler)

	//check result

	customActionPlan, err := customactionplan.New(protSettings, currentPackageRegistry, ext.HandlerEnv, ext.ExtensionLogger)
	if err != nil {
		return "could not create custom action action plan", err
	}
	_, customActionResults := customActionPlan.Execute(ext.ExtensionEvents, &commandHandler, vmAppResults)
	return getStatusMessage(currentPackageRegistry.GetPackageCollection(), customActionResults), executeError.GetErrorIfDeploymentFailed()
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
