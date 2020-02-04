package vmextensionhelper

import (
	"os"
	"os/exec"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type mockGetVMExtensionEnvironmentManager struct {
	seqNo                         uint
	currentSeqNo                  uint
	he                            HandlerEnvironment
	hs                            HandlerSettings
	getHandlerEnvironmentError    error
	findSeqNumError               error
	getCurrentSequenceNumberError error
	getHandlerSettingsError       error
	setSequenceNumberError        error
}

func (mm mockGetVMExtensionEnvironmentManager) getHandlerEnvironment(name string, version string) (he HandlerEnvironment, _ error) {
	if mm.getHandlerEnvironmentError != nil {
		return he, mm.getHandlerEnvironmentError
	}

	return mm.he, nil
}

func (mm mockGetVMExtensionEnvironmentManager) findSeqNum(ctx log.Logger, configFolder string) (uint, error) {
	if mm.findSeqNumError != nil {
		return 0, mm.findSeqNumError
	}

	return mm.seqNo, nil
}

func (mm mockGetVMExtensionEnvironmentManager) getCurrentSequenceNumber(ctx log.Logger, retriever sequenceNumberRetriever, name string, version string) (uint, error) {
	if mm.getCurrentSequenceNumberError != nil {
		return 0, mm.getCurrentSequenceNumberError
	}

	return mm.currentSeqNo, nil
}

func (mm mockGetVMExtensionEnvironmentManager) getHandlerSettings(ctx log.Logger, he HandlerEnvironment, seqNo uint) (hs HandlerSettings, _ error) {
	if mm.getHandlerSettingsError != nil {
		return hs, mm.getHandlerSettingsError
	}

	return mm.hs, nil
}

func (mm mockGetVMExtensionEnvironmentManager) setSequenceNumberInternal(ve *VMExtension, seqNo uint) error {
	if mm.setSequenceNumberError != nil {
		return mm.setSequenceNumberError
	}

	return nil
}

func Test_getVMExtensionNilValues(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	_, err := GetVMExtension(ctx, nil)
	require.Equal(t, ErrArgCannotBeNull, err)

	initInfo := &InitializationInfo{Name: ""}
	_, err = GetVMExtension(ctx, initInfo)
	require.Equal(t, ErrArgCannotBeNullOrEmpty, err)

	initInfo = &InitializationInfo{Name: "yaba", Version: ""}
	_, err = GetVMExtension(ctx, initInfo)
	require.Equal(t, ErrArgCannotBeNullOrEmpty, err)

	initInfo = &InitializationInfo{Name: "yaba", Version: "1.0", EnableCallback: nil}
	_, err = GetVMExtension(ctx, initInfo)
	require.Equal(t, ErrArgCannotBeNull, err)
}

func Test_getVMExtensionGetHandlerEnvironmentError(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	myerr := errors.New("cannot handle the environment")

	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	mm := mockGetVMExtensionEnvironmentManager{getHandlerEnvironmentError: myerr}
	_, err := getVMExtensionInternal(ctx, ii, mm)
	require.Equal(t, myerr, err)
}

func Test_getVMExtensionCannotFindSeqNo(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	mm.findSeqNumError = errors.New("the sequence number annoys me")
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)

	_, err := getVMExtensionInternal(ctx, ii, mm)
	require.Error(t, err)
}

func Test_getVMExtensionCannotReadCurrentSeqNo(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	mm.getCurrentSequenceNumberError = errors.New("the current sequence number is beyond our comprehension")
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)

	_, err := getVMExtensionInternal(ctx, ii, mm)
	require.Error(t, err)
}

func Test_getVMExtensionUpdateSupport(t *testing.T) {
	// Update disabled
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ext, err := getVMExtensionInternal(ctx, ii, mm)
	require.NoError(t, err, "getVMExtensionInternal failed")
	require.NotNil(t, ext)

	// Verify this is a noop
	updateNormalCallbackCalled = false
	cmd := ext.exec.cmds["update"]
	require.NotNil(t, cmd)
	_, err = cmd.f(ctx, ext)
	require.NoError(t, err, "updateCallback failed")
	require.False(t, updateNormalCallbackCalled)

	// Update enabled
	ii.UpdateCallback = testUpdateCallbackNormal
	ext, err = getVMExtensionInternal(ctx, ii, mm)
	require.NoError(t, err, "getVMExtensionInternal failed")
	require.NotNil(t, ext)

	// Verify this is not a noop
	cmd = ext.exec.cmds["update"]
	require.NotNil(t, cmd)
	_, err = cmd.f(ctx, ext)
	require.NoError(t, err, "updateCallback failed")
	require.True(t, updateNormalCallbackCalled)
}

func Test_getVMExtensionDisableSupport(t *testing.T) {
	// Disbable disabled
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ii.SupportsDisable = false
	ext, err := getVMExtensionInternal(ctx, ii, mm)
	require.NoError(t, err, "getVMExtensionInternal failed")
	require.NotNil(t, ext)

	// Verify this is a noop
	err = setDisabled(ctx, ext, false)
	require.NoError(t, err, "setDisabled failed")
	cmd := ext.exec.cmds["disable"]
	require.NotNil(t, cmd)
	_, err = cmd.f(ctx, ext)
	require.NoError(t, err, "disable cmd failed")
	require.False(t, isDisabled(ctx, ext))

	// Diable enabled
	ii.SupportsDisable = true
	ext, err = getVMExtensionInternal(ctx, ii, mm)
	require.NoError(t, err, "getVMExtensionInternal failed")
	require.NotNil(t, ext)

	// Verify this is not a noop
	cmd = ext.exec.cmds["disable"]
	require.NotNil(t, cmd)
	_, err = cmd.f(ctx, ext)
	defer setDisabled(ctx, ext, false)
	require.NoError(t, err, "disable cmd failed")
	require.True(t, isDisabled(ctx, ext))
}

func Test_getVMExtensionCannotGetSettings(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	mm.getHandlerSettingsError = errors.New("the settings exist only in a parallel dimension")
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)

	_, err := getVMExtensionInternal(ctx, ii, mm)
	require.Equal(t, mm.getHandlerSettingsError, err)
}

func Test_getVMExtensionNormalOperation(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)

	ext, err := getVMExtensionInternal(ctx, ii, mm)
	require.NoError(t, err, "getVMExtensionInternal failed")
	require.NotNil(t, ext)
}

func Test_parseCommandWrongArgsCount(t *testing.T) {
	if os.Getenv("DIE_PROCESS_DIE") == "1" {
		ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
		mm := createMockVMExtensionEnvironmentManager()
		ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
		ext, _ := getVMExtensionInternal(ctx, ii, mm)

		args := make([]string, 1)
		args[0] = "install"
		ext.parseCmd(args)
		return
	}

	// Verify that the process exits
	cmd := exec.Command(os.Args[0], "-test.run=Test_parseCommandWrongArgsCount")
	cmd.Env = append(os.Environ(), "DIE_PROCESS_DIE=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

func Test_parseCommandUnsupportedOperation(t *testing.T) {
	if os.Getenv("DIE_PROCESS_DIE") == "1" {
		ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
		mm := createMockVMExtensionEnvironmentManager()
		ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
		ext, _ := getVMExtensionInternal(ctx, ii, mm)

		args := make([]string, 2)
		args[0] = "processname_dont_care"
		args[1] = "flipperdoodle"
		ext.parseCmd(args)
		return
	}

	// Verify that the process exits
	cmd := exec.Command(os.Args[0], "-test.run=Test_parseCommandUnsupportedOperation")
	cmd.Env = append(os.Environ(), "DIE_PROCESS_DIE=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

func Test_parseCommandNormalOperation(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ext, _ := getVMExtensionInternal(ctx, ii, mm)

	args := make([]string, 2)
	args[0] = "processname_dont_care"
	args[1] = "install"
	cmd := ext.parseCmd(args)
	require.NotNil(t, cmd)
}

func Test_enableNoSeqNoChangeButRequired(t *testing.T) {
	if os.Getenv("DIE_PROCESS_DIE") == "1" {
		ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
		mm := createMockVMExtensionEnvironmentManager()
		mm.currentSeqNo = mm.seqNo
		ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
		ii.RequiresSeqNoChange = true
		ext, _ := getVMExtensionInternal(ctx, ii, mm)

		enable(ctx, ext)
		os.Exit(2) // enable above should exit the process cleanly. If it doesn't, fail.
	}

	// Verify that the process exits
	cmd := exec.Command(os.Args[0], "-test.run=Test_enableNoSeqNoChangeButRequired")
	cmd.Env = append(os.Environ(), "DIE_PROCESS_DIE=1")
	err := cmd.Run()
	if _, ok := err.(*exec.ExitError); !ok {
		return
	}
	t.Fatal("Process didn't shut cleanly as expected")
}

func Test_reenableExtension(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ii.SupportsDisable = true
	ext, _ := getVMExtensionInternal(ctx, ii, mm)

	err := setDisabled(ctx, ext, true)
	defer setDisabled(ctx, ext, false)
	require.NoError(t, err, "setDisabled failed")
	_, err = enable(ctx, ext)
	require.NoError(t, err, "enable failed")
	require.False(t, isDisabled(ctx, ext))
}

func Test_reenableExtensionFails(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ii.SupportsDisable = true
	ext, _ := getVMExtensionInternal(ctx, ii, mm)

	err := setDisabled(ctx, ext, true)
	defer setDisabled(ctx, ext, false)
	require.NoError(t, err, "setDisabled failed")
	disableDependency = evilDisableDependencies{}
	defer resetDependencies()
	msg, err := enable(ctx, ext)
	require.NoError(t, err) // We let the extension continue if we fail to reenable it
	require.Equal(t, "blah", msg)
}

func Test_enableCallbackFails(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testFailEnableCallback)
	ext, _ := getVMExtensionInternal(ctx, ii, mm)

	_, err := enable(ctx, ext)
	require.Equal(t, ErrMustRunAsAdmin, err)
}

func Test_enableCallbackSucceeds(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ext, _ := getVMExtensionInternal(ctx, ii, mm)

	msg, err := enable(ctx, ext)
	require.NoError(t, err, "enable failed")
	require.Equal(t, "blah", msg)
}

func Test_doFailToWriteSequenceNumber(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	mm.setSequenceNumberError = ErrMustRunAsAdmin
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ext, _ := getVMExtensionInternal(ctx, ii, mm)

	// We log but continue if we fail to write the sequence number
	oldArgs := os.Args
	defer putBackArgs(oldArgs)
	os.Args = make([]string, 2)
	os.Args[0] = "dontcare"
	os.Args[1] = "enable"
	ext.Do(ctx)
}

func Test_doCommandFails(t *testing.T) {
	if os.Getenv("DIE_PROCESS_DIE") == "1" {
		ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
		mm := createMockVMExtensionEnvironmentManager()
		ii, _ := GetInitializationInfo("yaba", "5.0", true, testFailEnableCallback)
		ext, _ := getVMExtensionInternal(ctx, ii, mm)

		oldArgs := os.Args
		defer putBackArgs(oldArgs)
		os.Args = make([]string, 2)
		os.Args[0] = "dontcare"
		os.Args[1] = "enable"
		ext.Do(ctx)
		return
	}

	// Verify that the process exits
	cmd := exec.Command(os.Args[0], "-test.run=Test_doCommandFails")
	cmd.Env = append(os.Environ(), "DIE_PROCESS_DIE=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 3", err)
}

func Test_doCommandSucceeds(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mm := createMockVMExtensionEnvironmentManager()
	ii, _ := GetInitializationInfo("yaba", "5.0", true, testEnableCallback)
	ext, _ := getVMExtensionInternal(ctx, ii, mm)

	oldArgs := os.Args
	defer putBackArgs(oldArgs)
	os.Args = make([]string, 2)
	os.Args[0] = "dontcare"
	os.Args[1] = "enable"
	ext.Do(ctx)
}

func putBackArgs(args []string) {
	os.Args = args
}

func testFailEnableCallback(ctx log.Logger, ext *VMExtension) (string, error) {
	return "", ErrMustRunAsAdmin
}

func createMockVMExtensionEnvironmentManager() mockGetVMExtensionEnvironmentManager {
	publicSettings := make(map[string]interface{})
	publicSettings["Flipper"] = "flip"
	publicSettings["Flopper"] = "flop"
	hs := HandlerSettings{PublicSettings: publicSettings, ProtectedSettings: nil}
	he := getTestHandlerEnvironment()

	return mockGetVMExtensionEnvironmentManager{
		seqNo:        5,
		currentSeqNo: 4,
		hs:           hs,
		he:           he,
	}
}
