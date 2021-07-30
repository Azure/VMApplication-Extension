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

func TestSingleCustomActionWithParameterLinux(t *testing.T) {
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
	cmdHandler := NewCommandHandlerMock(mockCommandExecutorNoError)
	appPackage, err := packageReg.GetExistingPackages()

	_, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)
	
	packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
	assert.True(t, ok)
	assertTickCountFileCorrect(t, strconv.FormatUint(action[0].Actions[0].TickCount, 10))
	fmt.Println((*packageOperationResults))
	assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: Success, Operation: "action1", AppVersion: "1.0", PackageName: newApp.ApplicationName, Timestamp: "20210604T155300Z"})
}

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
	// this sleep should never be hit
	time.Sleep(5 * time.Second)
	return 0, nil
}


func executeTestInAnotherThreadAndTerminateBeforeCompletion(t *testing.T, testName, packageDir, transcriptFile string) {
	initializeTest(t)
	file, err := os.Create(transcriptFile)
	assert.NoError(t, err, "should be able to create transcript file")
	if err == nil {
		defer file.Close()
	}
	c := exec.Command("go", "test", "-v", packageDir, "-run", testName)
	c.Dir = packageDir
	c.Env = append(os.Environ(), fmt.Sprintf("%s=true", LaunchedFromAnotherProcessEnvVariable))
	c.Stdout = file
	c.Run()
}

func TestCommandExecutorCanHandleProcessBeingKilled(t *testing.T) {
	envVariables := os.Environ()
	var wasStartedByAnotherProcess= false
	for _, variable := range envVariables {
		if strings.Contains(variable, LaunchedFromAnotherProcessEnvVariable) {
			wasStartedByAnotherProcess = true
		}
	}

	action := []*VmAppSetting{
		{
			ApplicationName: "app1",
			Order:           &one,
			Actions: []*ActionSetting{
				{
					ActionName:   "action1",
					ActionScript: "echo hello",
					Timestamp:    "20210604T155300Z",
					Parameters:   []ActionParameter{},
					TickCount:    10193113,
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

	if wasStartedByAnotherProcess {
		initializeTest(t)
		packageReg, err := packageregistry.New(environment, time.Second)
		assert.NoError(t, err)
		if err == nil {
			defer packageReg.Close()
		}
		err = packageReg.WriteToDisk(newRegistry)
		assert.NoError(t, err)
		appPackage, err := packageReg.GetExistingPackages()

		cmdHandler := NewCommandHandlerMock(mockCommandExecutorKillProcess)
		_, statusMessage := executeActionPlan(t, action, appPackage, cmdHandler)
		packageOperationResults, ok := statusMessage.(*actionplan.PackageOperationResults)
		assert.True(t, ok)
		assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
		assert.EqualValues(t, (*packageOperationResults)[0], actionplan.PackageOperationResult{Result: Success, Operation: "action1", AppVersion: newApp.Version, PackageName: newApp.ApplicationName})

	} else {
		defer cleanupTest()
		currentDirAbsolutePath, err := filepath.Abs("")
		assert.NoError(t, err, "should be able to get absolute path")
		transcriptFile := path.Join(currentDirAbsolutePath, testdir, "transcript.txt")
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled", currentDirAbsolutePath, transcriptFile)
		assertTickCountFileCorrect(t, action[0].Actions[0].TickCount)
	}

}