package main

import (
	"os"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/VMApplication-Extension/pkg/commandhandler"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/go-kit/kit/log"
)

// Note: not const so test can change them
var (
	extensionName    = "Microsoft.Azure.Extensions.VMApp"
	extensionVersion = "1.0.0"

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
	logger := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ctx := log.With(log.With(logger, "time", log.DefaultTimestampUTC), "version", VersionString())
	// require SeqNoChange is set to false because we want the extension to ensure that the packages are in sync with the desired packages
	ii, err := vmextensionhelper.GetInitializationInfo(extensionName, extensionVersion, false, vmAppEnableCallback)
	if err != nil {
		ctx.Log("event", "Failed to create initialization info", "error", err)
		return err
	}

	ext, err := vmextensionhelper.GetVMExtension(ctx, ii)
	if err != nil {
		ctx.Log("event", "Failed to create extension info", "error", err)
		return err
	}

	ext.Do(ctx)

	ctx.Log("event", "end")

	return nil
}

// Callback indicating the operation is enable and the sequence number has changed
func vmAppEnableCallback(ctx log.Logger, ext *vmextensionhelper.VMExtension) (string, error) {
	hostGaCommunicator := hostgacommunicator.HostGaCommunicator{}
	return doVmAppEnableCallback(ctx, ext, &hostGaCommunicator)
}

func doVmAppEnableCallback(ctx log.Logger, ext *vmextensionhelper.VMExtension, hostGaCommunicator hostgacommunicator.IHostGaCommunicator) (string, error) {
	vmAppIncomingCollection, err := getVMAppIncomingCollection(ext.Settings, hostGaCommunicator, ctx)
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

	actionPlan, err := actionplan.New(currentPackageRegistry, vmAppIncomingCollection, ext.HandlerEnv, hostGaCommunicator, ctx)
	if err != nil {
		return "could not create action plan", err
	}

	commandHandler := commandhandler.CommandHandler{}

	err = actionPlan.Execute(packageRegistry, &commandHandler)

	if err != nil {
		// actionPlan.Execute can fail partially
		// return ths string that contains operations that failed, but mark the overall process as success
		return err.Error(), nil
	}

	return "Operation completed", nil
}
