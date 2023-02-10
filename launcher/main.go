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
)

var ( // set at compile time
	ExtensionVersion string
	ExtensionName    = constants.ExtensionName
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

	if ExtensionName == "" || ExtensionVersion == "" {
		el.Error("variables not set at compile time, program needs recompilation")
		eh.Exit(exithelper.MiscError)
	}

	arg := args[1]

	switch arg {
	case "name":
		fmt.Println(ExtensionName)
		return
	case "version":
		fmt.Println(ExtensionVersion)
		return
	}

	handlerEnv, err := handlerEnvironmentGetter(ExtensionName, ExtensionVersion)
	if err != nil {
		el.Error("could not retrieve handler environment %s", err.Error())
		eh.Exit(exithelper.EnvironmentError)
	}
	el = logging.New(handlerEnv)
	currentSequenceNumber, err := seqno.GetCurrentSequenceNumber(el, currentSeqnoRetriever, ExtensionName, ExtensionVersion)
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
			el.Error(err.Error())
			extensionEvents.LogErrorEvent("Save Status", err.Error())
		}
		eh.Exit(exithelper.FileSystemError)
	}
}
