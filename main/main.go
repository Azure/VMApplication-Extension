package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

// Note: not const so test can change them
var (
	extensionVersion = "1.0.6"
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
	ii, err := vmextensionhelper.GetInitializationInfo(extensionName, extensionVersion, false, vmAppEnableCallback)
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

	ext.Do()

	return nil
}

// Callback indicating the operation is enable
func vmAppEnableCallback(ext *vmextensionhelper.VMExtension) (string, error) {
	ext.ExtensionEvents.LogInformationalEvent("Starting", "VmApplications extension starting")
	hostGaCommunicator := hostgacommunicator.HostGaCommunicator{}
	result, err := doVmAppEnableCallback(ext, &hostGaCommunicator)
	if err == nil {
		ext.ExtensionEvents.LogInformationalEvent("Completed", "VmApplications extension finished. Result=Success")
	} else {
		ext.ExtensionEvents.LogInformationalEvent(
			"Completed",
			fmt.Sprintf("VmApplications extension finished. Result=Failure;Reason=%v", err.Error()))
	}

	return result, err
}

func doVmAppEnableCallback(ext *vmextensionhelper.VMExtension, hostGaCommunicator hostgacommunicator.IHostGaCommunicator) (string, error) {
	settings, err := ext.GetSettings()
	if err != nil {
		return "could not get extension settings", err
	}
	vmAppIncomingCollection, err := getVMAppIncomingCollection(settings, hostGaCommunicator, ext.ExtensionLogger)
	if err != nil {
		return "resolving packages failed", err
	}
	packageRegistry, err := packageregistry.New(ext.ExtensionLogger, ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		return "could not create package registry", err
	}
	defer packageRegistry.Close()
	currentPackageRegistry, err := packageRegistry.GetExistingPackages()
	if err != nil {
		return "could not read current package registry", err
	}

	actionPlan := actionplan.New(currentPackageRegistry, vmAppIncomingCollection, ext.HandlerEnv, hostGaCommunicator, ext.ExtensionLogger)
	commandHandler := commandhandler.CommandHandler{}

	// actionPlan.Execute can fail partially, but we mark the overall process as success
	// errors are sent in the status message
	_, result := actionPlan.Execute(packageRegistry, ext.ExtensionEvents, &commandHandler)

	currentPackageRegistry, err = packageRegistry.GetExistingPackages()
	if err != nil {
		return "could not read current package registry", err
	}

	return getStatusMessage(currentPackageRegistry.GetPackageCollection(), result), nil
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
