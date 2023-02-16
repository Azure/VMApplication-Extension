package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-extension-platform/pkg/commandhandler"
	"github.com/Azure/azure-extension-platform/pkg/logging"
)

var commandHandlerToUse = commandhandler.New()

func runExecutableAsIndependentProcess(exeName, args, workingDir, logDir string, el *logging.ExtensionLogger) {
	relativeFilePath := filepath.Dir(os.Args[0])
	exefullPathAndName := filepath.Join(relativeFilePath, exeName)
	commandToExecute := fmt.Sprintf("%s %s", exefullPathAndName, args)
	commandHandlerToUse.Execute(commandToExecute, workingDir, logDir, false, el)
}
