package customactionplan

import (
	"fmt"
	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

func TestSingleCustomActionWithParameterWindows(t *testing.T) {
	initializeTest(t)
	defer cleanupTest()
	action := []*VmAppSetting {
		{
			ApplicationName: "app1",
			Order: &one,
			Actions: []*ActionSetting{
				{
					ActionName: "action1",
					ActionScript: "echo %CustomAction_FOO%",
					Timestamp: "20210604T155300Z",
					Parameters: []ActionParameter{
						{
							ParameterName: "FOO",
							ParameterValue: "Hello World",
						},
					},
					TickCount: 10193113,
				},
			},
		},
	}
	newApp := packageregistry.VMAppPackageCurrent{
		ApplicationName: "app1",
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	newRegistry := packageregistry.CurrentPackageRegistry{
		"app1": &newApp,
	}
	packageReg, err := packageregistry.New(environment, time.Second)
	assert.NoError(t, err)
	if err == nil {
		defer packageReg.Close()
	}
	err = packageReg.WriteToDisk(newRegistry)
	assert.NoError(t, err)
	//existingApps := packageregistry.VMAppPackageCurrentCollection{}
	//incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	appPackage, err := packageReg.GetExistingPackages()
	//for k := range appPackage {
	//	fmt.Println(k)
	//}
	_, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)

	//assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
	//assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
	//assertAllActionsSucceeded(t, newReg)
	packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
	assert.True(t, ok)
	assertTickCountFileCorrect(t, strconv.FormatUint(action[0].Actions[0].TickCount, 10))
	fmt.Println((*packageOperationResults))
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp.ApplicationName, Timestamp: "20210604T155300Z"})
}
