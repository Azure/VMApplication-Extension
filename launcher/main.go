package main

import (
	"fmt"
	"os"

	"github.com/Azure/VMApplication-Extension/internal/constants"
	"github.com/Azure/VMApplication-Extension/pkg/utils"
	"github.com/Azure/azure-extension-platform/pkg/exithelper"
	"github.com/Azure/azure-extension-platform/pkg/extensionevents"
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/Azure/azure-extension-platform/pkg/seqno"
	"github.com/Azure/azure-extension-platform/pkg/status"
	platformUtils "github.com/Azure/azure-extension-platform/pkg/utils"
)

var ( // set at compile time
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
	requestedSeqnoRetriever  func(el *logging.ExtensionLogger, configFolder string) (uint, error)    = seqno.FindSeqNum
)

func main() {
	if len(args) != 2 {
		el.Error("requires at least one argument")
		eh.Exit(exithelper.ArgumentError)
	}

	// check compile time values
	if ExecutableName == "" || ExtensionVersion == "" {
		el.Error("variables not set at compile time, program needs recompilation")
		eh.Exit(exithelper.MiscError)
	}

	arg := args[1]

	handlerEnv, err := handlerEnvironmentGetter(constants.ExtensionName, ExtensionVersion)
	if err != nil {
		el.Error("could not retrieve handler environment %s", err.Error())
		eh.Exit(exithelper.EnvironmentError)
	}
	el = logging.New(handlerEnv)
	currentSequenceNumber, err := seqno.GetCurrentSequenceNumber(el, currentSeqnoRetriever, constants.ExtensionName, ExtensionVersion)
	if err != nil {
		el.Error("could not determine current sequence number: %v", err)
		eh.Exit(exithelper.EnvironmentError)
	}
	requestedSequenceNumber, err := requestedSeqnoRetriever(el, handlerEnv.ConfigFolder)
	if err != nil {
		el.Error("could not determine current sequence number: %v", err)
		eh.Exit(exithelper.EnvironmentError)
	}
	extensionEvents := extensionevents.New(el, handlerEnv)

	if requestedSequenceNumber > currentSequenceNumber {
		// only write transitioning status file for new sequence numbers
		err = utils.ReportStatus(handlerEnv, requestedSequenceNumber, status.StatusTransitioning, arg, "transitioning")
		if err != nil {
			el.Error(fmt.Sprintf("could not write transitioning status: %s", err.Error()))
			extensionEvents.LogCriticalEvent("Save Status", err.Error())
		}
		eh.Exit(exithelper.FileSystemError)
	}

	currentDir, err := platformUtils.GetCurrentProcessWorkingDir()
	if err != nil {
		el.Error(fmt.Sprintf("Could not determine current process working directory %s", err.Error()))
		extensionEvents.LogCriticalEvent("Get Current Process Working Directory", err.Error())
	}
	runExecutableAsIndependentProcess(ExecutableName, arg, currentDir, handlerEnv.LogFolder, el)
}
