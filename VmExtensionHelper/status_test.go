package vmextensionhelper

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

var (
	statusTestDirectory = "./statustest"
)

func Test_statusMsgSucceededWithString(t *testing.T) {
	s := statusMsg("yaba", StatusSuccess, "")
	require.Equal(t, "yaba succeeded", s)
}

func Test_statusMsgFailedWithMsg(t *testing.T) {
	s := statusMsg("", StatusError, "flipper")
	require.Equal(t, " failed: flipper", s)
}

func Test_statusMsgInProgressEmpty(t *testing.T) {
	s := statusMsg("", StatusTransitioning, "")
	require.Equal(t, " in progress", s)
}

func Test_statusMsgOther(t *testing.T) {
	s := statusMsg("yaba", "flooper", "flop")
	require.Equal(t, "yaba: flop", s)
}

func Test_statusMsgFull(t *testing.T) {
	s := statusMsg("yaba", StatusSuccess, "flop")
	require.Equal(t, "yaba succeeded: flop", s)
}

func Test_newStatus(t *testing.T) {
	report := NewStatus(StatusError, "WorldDomination", "bow before the unit test!")
	require.NotNil(t, report)
	require.Equal(t, 1, len(report))
	require.Equal(t, "WorldDomination", report[0].Status.Operation)
	require.Equal(t, StatusError, report[0].Status.Status)
}

func Test_statusSaveFolderDoesntExist(t *testing.T) {
	report := NewStatus(StatusSuccess, "flip", "flop")
	err := report.Save("./flopperdoodle", 5)
	require.Error(t, err)
}

func Test_statusSaveNewFile(t *testing.T) {
	report := NewStatus(StatusSuccess, "flip", "flop")
	cleanupTestDirectory(t, statusTestDirectory)
	err := report.Save(statusTestDirectory, 5)
	require.NoError(t, err, "save report failed")

	filePath := path.Join(statusTestDirectory, "5.status")
	b, err := ioutil.ReadFile(filePath)
	require.NoError(t, err, "ReadFile failed")

	var r StatusReport
	err = json.Unmarshal(b, &r)
	require.NoError(t, err, "Unmarshal failed")
	require.Equal(t, 1, len(r))
	require.Equal(t, "flip", report[0].Status.Operation)
	require.Equal(t, StatusSuccess, report[0].Status.Status)
}

func Test_statusSaveExistingFile(t *testing.T) {
	report := NewStatus(StatusSuccess, "flip", "flop")
	cleanupTestDirectory(t, statusTestDirectory)
	err := report.Save(statusTestDirectory, 7)
	require.NoError(t, err, "save report failed")
	err = report.Save(statusTestDirectory, 7)
	require.NoError(t, err, "second ave report failed")
}

func Test_reportStatusShouldntReport(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()
	c := cmd{nil, "Install", false, 99}
	ext.HandlerEnv.StatusFolder = statusTestDirectory
	ext.RequestedSequenceNumber = 45

	err := reportStatus(ctx, ext, StatusSuccess, c, "msg")
	require.NoError(t, err, "reportStatus failed")
	_, err = os.Stat(path.Join(statusTestDirectory, "45.status"))
	require.True(t, os.IsNotExist(err), "File exists when we don't expect it to")
}

func Test_reportStatusCouldntSave(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()
	c := cmd{nil, "Install", true, 99}
	ext.HandlerEnv.StatusFolder = "./yabamonster"
	ext.RequestedSequenceNumber = 45

	err := reportStatus(ctx, ext, StatusSuccess, c, "msg")
	require.Error(t, err)
}

func Test_reportStatusSaved(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()
	c := cmd{nil, "Install", true, 99}
	ext.HandlerEnv.StatusFolder = statusTestDirectory
	ext.RequestedSequenceNumber = 45

	err := reportStatus(ctx, ext, StatusSuccess, c, "msg")
	require.NoError(t, err, "reportStatus failed")
	_, err = os.Stat(path.Join(statusTestDirectory, "45.status"))
	require.NoError(t, err, "File doesn't exist")
}
