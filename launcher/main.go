// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"os"

	"github.com/Azure/VMApplication-Extension/pkg/utils"
	platformconstants "github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/Azure/azure-extension-platform/pkg/exithelper"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/Azure/azure-extension-platform/pkg/seqno"
	"github.com/Azure/azure-extension-platform/pkg/status"
	platformutils "github.com/Azure/azure-extension-platform/pkg/utils"
	"github.com/Azure/azure-extension-platform/vmextension"
)

var ( // set at compile time
	ExtensionName    string
	ExtensionVersion string
	ExecutableName   string
)

var (
	el = logging.New(nil)
	eh = exithelper.Exiter
)

var ( // variables that can be overwritten for testing
	args                     []string                                                                = os.Args
	handlerEnvironmentGetter func(name, version string) (he *handlerenv.HandlerEnvironment, _ error) = handlerenv.GetHandlerEnvironment
	currentSeqnoRetriever    seqno.ISequenceNumberRetriever                                          = &seqno.ProdSequenceNumberRetriever{}
	requestedSeqnoRetriever  func(el logging.ILogger, configFolder string) (uint, error)             = seqno.FindSeqNum
)

func main() {
	if len(args) != 2 {
		el.Error("requires at least one argument")
		eh.Exit(exithelper.ArgumentError)
	}

	// check compile time values
	if ExecutableName == "" || ExtensionName == "" || ExtensionVersion == "" {
		el.Error("variables not set at compile time, program needs recompilation")
		eh.Exit(exithelper.MiscError)
	}

	// validate ExtensionVersion against the version reported by Guest Agent
	if guestAgentVersion, err := vmextension.GetGuestAgentEnvironmetVariable(vmextension.GuestAgentEnvVarExtensionVersion); err == nil {
		if guestAgentVersion != ExtensionVersion {
			el.Warn("ExtensionVersion mismatch: compile-time ExtensionVersion value '%s' does not match value '%s' in environment variable '%s'", ExtensionVersion, guestAgentVersion, vmextension.GuestAgentEnvVarExtensionVersion)
		}
	}

	arg := args[1]

	switch arg {
	case "version":
		fmt.Printf("Extension version is %s%s", ExtensionVersion, platformconstants.NewLineCharacter)
		return
	case "exename":
		fmt.Printf("Executable name is %s%s", ExecutableName, platformconstants.NewLineCharacter)
		return
	case vmextension.EnableOperation.ToString():
		break
	default:
		fmt.Println("Only valid arguments are 'version', 'exename', 'enable'. Exiting")
		eh.Exit(exithelper.ArgumentError)
	}

	handlerEnv, err := handlerEnvironmentGetter(ExtensionName, ExtensionVersion)
	if err != nil {
		el.Error("Could not retrieve handler environment %s", err.Error())
		eh.Exit(exithelper.EnvironmentError)
	}
	el = logging.New(handlerEnv)
	currentSequenceNumber, err := seqno.GetCurrentSequenceNumber(el, currentSeqnoRetriever, ExtensionName, ExtensionVersion)
	if err != nil {
		el.Error("Could not determine current sequence number: %v", err)
		eh.Exit(exithelper.EnvironmentError)
	}
	requestedSequenceNumber, err := requestedSeqnoRetriever(el, handlerEnv.ConfigFolder)
	if err != nil {
		el.Error("Could not determine requested sequence number: %v", err)
		eh.Exit(exithelper.EnvironmentError)
	}
	extensionEvents := extensionevents.New(el, handlerEnv)

	if requestedSequenceNumber >= currentSequenceNumber {
		// attempt to write a transitioning status file if it doesn't exist
		_, getStatusError := utils.GetStatusType(handlerEnv, requestedSequenceNumber)
		if getStatusError != nil {
			// either no transitioning status file was found, or the status file was malformed
			// either way create a new transitioning status file
			err = utils.ReportStatus(handlerEnv, requestedSequenceNumber, status.StatusTransitioning, arg, "transitioning")
			if err != nil {
				el.Error(fmt.Sprintf("Could not write transitioning status: %s", err.Error()))
				extensionEvents.LogCriticalEvent("Save Status", err.Error())
				eh.Exit(exithelper.FileSystemError)
			}
			el.Info("Wrote transitioning status file for sequence number %d", requestedSequenceNumber)
		}
	}

	currentDir, err := platformutils.GetCurrentProcessWorkingDir()
	if err != nil {
		el.Error(fmt.Sprintf("Could not determine current process working directory %s", err.Error()))
		extensionEvents.LogCriticalEvent("Get Current Process Working Directory", err.Error())
	}
	runExecutableAsIndependentProcess(ExecutableName, arg, currentDir, handlerEnv.LogFolder, el)
}
