package main

import (
	"fmt"
	"path/filepath"

	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/Azure/azure-extension-platform/pkg/utils"
)

var commandHandlerToUse = commandhandler.New()

func runExecutableAsIndependentProcess(exeName, args, workingDir, logDir string, el *logging.ExtensionLogger) {
	exepath, err := utils.GetCurrentProcessWorkingDir()
	if err != nil {
		el.Error("could not determine current process working directory %s", err.Error())
		return
	}
	exefullPathAndName := filepath.Join(exepath, exeName)
	commandToExecute := fmt.Sprintf("%s %s", exefullPathAndName, args)
	commandHandlerToUse.Execute(commandToExecute, workingDir, logDir, false, el)
}
