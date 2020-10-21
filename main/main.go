package main

import (
	"encoding/json"
	"fmt"
	vmextensionhelper "github.com/D1v38om83r/azure-extension-platform/vmextension"
	"github.com/go-kit/kit/log"
	"os"
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
	vmPackagesSetting = "vmPackages"
	operationInstall  = "install"
	operationUpdate   = "update"
	operationRemove   = "remove"
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

func (vmp vmPackage) isInstallOrUpdate() bool {
	if vmp.Operation == operationInstall || vmp.Operation == operationUpdate {
		return true
	}

	return false
}

func (vmp vmPackage) isInstall() bool {
	if vmp.Operation == operationInstall {
		return true
	}

	return false
}

func (vmp vmPackage) isRemove() bool {
	if vmp.Operation == operationRemove {
		return true
	}

	return false
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
	ii, err := vmextensionhelper.GetInitializationInfo(extensionName, extensionVersion, true, vmAppEnableCallback)
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
	packageData, err := getVMPackageData(ctx, ext)
	if err != nil {
		ctx.Log("message", "Could not read the VM package data")
		return "", err
	}

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, packageData)
	if err != nil {
		ctx.Log("message", "Could not create package state plan")
		return "", err
	}

	if requiresChanges || hasProposedState {
		return processPackages(ctx, ext)
	}

	return "Nothing to process", nil
}

func processPackages(ctx log.Logger, ext *vmextensionhelper.VMExtension) (string, error) {
	packageToProcess := getNextProposedPackage(ctx, ext)
	for packageToProcess != nil {
		err := processPackage(ctx, packageToProcess)
		if err != nil {
			ctx.Log("message", "Unable to process package '%s'", packageToProcess.Name)
		}

		err = markProposedPackageFinished(ctx, ext, packageToProcess)
		if err != nil {
			// If we fail to mark a package as finished, then we can no longer guarantee
			// the state by continuing to process, so game over
			ctx.Log("message", "Unable to mark package '%s' as finished", packageToProcess.Name)
			return "Failed to mark package", err
		}

		packageToProcess = getNextProposedPackage(ctx, ext)
	}

	return "Complete", nil
}

func processPackage(ctx log.Logger, packageToProcess *vmPackage) error {
	return nil
}

func getVMPackageData(ctx log.Logger, ext *vmextensionhelper.VMExtension) (*vmPackageData, error) {
	rawVMPackageData, ok := ext.Settings.ProtectedSettings[vmPackagesSetting]
	if !ok {
		ctx.Log("message", "operation not specified in settings")
		return nil, fmt.Errorf("Could not find '%s' in protected settings", vmPackagesSetting)
	}

	stringPackageData, ok := rawVMPackageData.(string)
	if !ok {
		ctx.Log("message", "Could not read package data")
		return nil, fmt.Errorf("Invalid VM package data")
	}

	b := []byte(stringPackageData)
	var packageData vmPackageData
	err := json.Unmarshal(b, &packageData)
	if err != nil {
		ctx.Log("message", "Could not unmarshal package data")
		return nil, fmt.Errorf("Invalid VM package data")
	}

	// Validate the applications
	for _, app := range packageData.Packages {
		if app.Name == "" {
			ctx.Log("message", "Application does not have a name")
			return nil, fmt.Errorf("Application does not have a name")
		}

		if app.Operation == "" {
			ctx.Log("message", "Application does not have an operation")
			return nil, fmt.Errorf("Application does not have an operation")
		}

		if app.Version == "" {
			ctx.Log("message", "Application does not have a version")
			return nil, fmt.Errorf("Application does not have a version")
		}

		// Set the sequence number, which we may serialize to disk. We need this for reporting
		app.SequenceNumber = ext.RequestedSequenceNumber
	}

	return &packageData, nil
}
