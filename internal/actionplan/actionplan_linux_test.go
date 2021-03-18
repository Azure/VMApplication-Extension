package actionplan

var mockCommandExecutorKillProcess CommandExecutor = func(s string, s2 string) (int, error) {
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		fmt.Print("could not find process")
	} else {
		err = proc.Signal(syscall.SIGTERM)
		if err != nil {
			fmt.Printf("could not kill process %s", err.Error())
		}
	}
	time.Sleep(1 * time.Minute)
	return 0, nil
}

func TestCommandExecutorCanHandleProcessBeingKilled(t *testing.T) {
	// this is a re-entrant test
	initializeTest(t)
	defer cleanupTest()
	newApp := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           &one,
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	existingApps := packageregistry.VMAppPackageCurrentCollection{}
	incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorKillProcess)

	var newReg packageregistry.CurrentPackageRegistry
	var statusMessage IStatusMessage
	done := make(chan bool, 1)
	go func(newReg *packageregistry.CurrentPackageRegistry, statusMessage *IStatusMessage) {
		*newReg, _, *statusMessage = executeActionPlan(t, existingApps, incomingApps, cmdHandler)
		done <- true
	}(&newReg, &statusMessage)
	<-done
	assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	assertAllActionsSucceeded(t, newReg)
	packageOperationResults, ok := statusMessage.(*PackageOperationResults)
	assert.True(t, ok)
	assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newApp.Version, PackageName: newApp.ApplicationName})
}

