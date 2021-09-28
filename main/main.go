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
)

// Note: not const so test can change them
var (
	extensionVersion = "1.0.4"

	// downloadDir is where we store the downloaded files in the "{downloadDir}/{seqnum}/file"
	// format and the logs as "{downloadDir}/{seqnum}/std(out|err)". Stored under dataDir
	downloadDir = "download"
)

const (
	vmPackagesSetting       = "vmPackages"
	operationInstall        = "install"
	operationUpdate         = "update"
	operationRemove         = "remove"
	filelockTimeoutDuration = 15 * time.Minute
)

type vmPackageData struct {
	Packages []vmPackage `json:"vmPackages"`
}

// Note that Name, Operation, and Version come from the protected settings sent by CRP
// SequenceNumber comes from the Guest Agent and is added by this code
// ProposedFileNumber is used only for proposal files. There is no need to serialize it.
type vmPackage struct {
	Name               string `json:"name"`
	Operation          string `json:"operation"`
	Version            string `json:"version"`
	SequenceNumber     uint   `json:"sequenceNumber"`
	ProposedFileNumber int
}

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
	
	ii.UpdateCallback = vmAppUpdateCallback

	ext, err := vmextensionhelper.GetVMExtension(ii)
	if err != nil {
		return err
	}

	ext.Do()

	return nil
}

// Callback indicating the operation is enable and the sequence number has changed
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
	packageRegistry, err := packageregistry.New(ext.HandlerEnv, filelockTimeoutDuration)
	if err != nil {
		return "could not create package registry", err
	}
	defer packageRegistry.Close()
	currentPackageRegistry, err := packageRegistry.GetExistingPackages()
	if err != nil {
		return "could not read current package registry", err
	}

	actionPlan, err := actionplan.New(currentPackageRegistry, vmAppIncomingCollection, ext.HandlerEnv, hostGaCommunicator, ext.ExtensionLogger)
	if err != nil {
		return "could not create action plan", err
	}

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
