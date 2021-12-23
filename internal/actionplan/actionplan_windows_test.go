package actionplan

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/Azure/azure-extension-platform/pkg/constants"
	"github.com/stretchr/testify/assert"
)

var mockCommandExecutorSleepForAnHour CommandExecutor = func(s string, s2 string) (int, error) {
	fmt.Sprint("sleeping for 1 hour")
	time.Sleep(1 * time.Hour)
	return 0, nil
}

func executeTestInAnotherThreadAndTerminateBeforeCompletion(t *testing.T, testName, packageDir, transcriptFile string, timeToWaitBeforeKilling time.Duration) {
	stdinBuffer := &bytes.Buffer{}
	stdoutBuffer := &bytes.Buffer{}
	c := exec.Command("powershell.exe")
	c.Dir = packageDir
	c.Stdin = stdinBuffer
	c.Env = append(os.Environ(), fmt.Sprintf("%s=true", LaunchedFromAnotherProcessEnvVariable))

	c.Stdout = stdoutBuffer

	ps1File := path.Join(packageDir, "test.ps1")

	commandToRunTest := fmt.Sprintf("Start-Transcript -Path %s; go test -v %s -run %s%s", transcriptFile, packageDir, testName, constants.NewLineCharacter)

	err := ioutil.WriteFile(ps1File, []byte(commandToRunTest), constants.FilePermissions_UserOnly_ReadWriteExecute)
	assert.NoError(t, err, "should be able to write command file")
	if err == nil {
		defer os.Remove(ps1File)
	}
	// write to Stdin to start the test
	stdinBuffer.WriteString(fmt.Sprintf("$p=(Start-Process powershell.exe %s -PassThru)%s$p.Id%s", ps1File, constants.NewLineCharacter, constants.NewLineCharacter))
	err = c.Start()
	assert.NoError(t, err, "should be able to start the executable")
	time.Sleep(timeToWaitBeforeKilling)

	stdoutBytes := make([]byte, 2056)
	_, err = stdoutBuffer.Read(stdoutBytes)
	assert.NoError(t, err, "should be able to read bytes")

	stringsbyLine := strings.Split(string(stdoutBytes), constants.NewLineCharacter)

	var lineContainingPid = -1
	for i, line := range stringsbyLine {
		matched, err := regexp.MatchString("^[0-9]+$", line)
		if err == nil && matched {
			lineContainingPid = i
			break
		}
	}

	if lineContainingPid == -1 {
		assert.Fail(t, "could not find line containing pid")
	}

	pid, err := strconv.ParseInt(stringsbyLine[lineContainingPid], 10, 32)
	assert.NoError(t, err, "should be able to parse %v to int", stringsbyLine[lineContainingPid])

	sendCtrlCToProcess(int(pid))
	err = c.Wait()
	assert.NoError(t, err)
}

func sendCtrlCToProcess(pid int) error {
	c := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "bypass", "-Command", fmt.Sprintf("Add-Type -Names 'w' -Name 'k' -M '[DllImport(\"kernel32.dll\")]public static extern bool FreeConsole();[DllImport(\"kernel32.dll\")]public static extern bool AttachConsole(uint p);[DllImport(\"kernel32.dll\")]public static extern bool SetConsoleCtrlHandler(uint h, bool a);[DllImport(\"kernel32.dll\")]public static extern bool GenerateConsoleCtrlEvent(uint e, uint p);public static void SendCtrlC(uint p){FreeConsole();AttachConsole(p);GenerateConsoleCtrlEvent(0, 0);}';[w.k]::SendCtrlC(%d)", pid))
	return c.Run()
}

func TestCommandExecutorCanHandleProcessBeingKilled(t *testing.T) {
	envVariables := os.Environ()
	var wasStartedByAnotherProcess = false
	for _, variable := range envVariables {
		if strings.Contains(variable, LaunchedFromAnotherProcessEnvVariable) {
			wasStartedByAnotherProcess = true
		}
	}
	newApp := packageregistry.VMAppPackageIncoming{
		ApplicationName: "app1",
		Order:           &one,
		Version:         "1.0",
		InstallCommand:  "install app1",
		RemoveCommand:   "remove app1",
		UpdateCommand:   "update app1",
	}
	if wasStartedByAnotherProcess {
		initializeTest(t)

		existingApps := packageregistry.VMAppPackageCurrentCollection{}
		incomingApps := packageregistry.VMAppPackageIncomingCollection{&newApp}
		cmdHandler := NewCommandHandlerMock(mockCommandExecutorSleepForAnHour)
		newReg, _, statusMessage := executeActionPlan(t, existingApps, incomingApps, cmdHandler)
		assert.EqualValues(t, newApp.InstallCommand, cmdHandler.Result[0].command, "Install command must be invoked")
		assertPackageRegistryHasBeenUpdatedProperly(t, newReg, incomingApps)
		assertAllActionsSucceeded(t, newReg)
		packageOperationResults, ok := statusMessage.(*PackageOperationResults)
		assert.True(t, ok)
		assert.EqualValues(t, (*packageOperationResults)[0], PackageOperationResult{Result: Success, Operation: packageregistry.Install.ToString(), AppVersion: newApp.Version, PackageName: newApp.ApplicationName})

	} else {
		defer cleanupTest()
		currentDirAbsolutePath, err := filepath.Abs("")
		assert.NoError(t, err, "should be able to get absolute path")
		transcriptFile := path.Join(currentDirAbsolutePath, testdir, "transcript.txt")
		// test takes at least 5 seconds to start, need to give it time before killing it
		executeTestInAnotherThreadAndTerminateBeforeCompletion(t, "TestCommandExecutorCanHandleProcessBeingKilled", currentDirAbsolutePath, transcriptFile, 10*time.Second)
		pkr, err := packageregistry.New(el, environment, time.Second)
		assert.NoError(t, err, "should be able to get current package registry")
		if err == nil {
			defer pkr.Close()
		}
		existingPackages, err := pkr.GetExistingPackages()
		assert.NoError(t, err, "should be able to get existing packages")
		app, ok := existingPackages[newApp.ApplicationName]
		assert.True(t, ok, "newApp should be present in current package registry")
		assert.Equal(t, packageregistry.NoAction, app.OngoingOperation)

		// wait for another 3 seconds to ensure that the transcript file is written
		time.Sleep(3 * time.Second)
		transcriptFileBytes, error := ioutil.ReadFile(transcriptFile)
		assert.NoError(t, error, "should be able to read transcript file")
		stranscriptFileString := string(transcriptFileBytes)
		assert.Contains(t, stranscriptFileString, "Info received terminate signal, system reboot detected")
	}
}
