package vmextensionhelper

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

var (
	sequenceNumberTestFolder = "./flooperflop"
)

type mockSequenceNumberRetriever struct {
	returnSeqNo uint
	returnError error
}

func (snr mockSequenceNumberRetriever) getSequenceNumber(name string, version string) (uint, error) {
	return snr.returnSeqNo, snr.returnError
}

func Test_getCurrentSequenceNumberNotFound(t *testing.T) {
	retriever := mockSequenceNumberRetriever{returnSeqNo: 0, returnError: errNotFound}
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	seqNo, err := getCurrentSequenceNumber(ctx, retriever, "yaba", "5.0")
	require.Equal(t, uint(0), seqNo)
	require.Nil(t, err)
}

func Test_getCurrentSequenceNumberOtherError(t *testing.T) {
	retriever := mockSequenceNumberRetriever{returnSeqNo: 0, returnError: ErrInvalidSettingsFile}
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	_, err := getCurrentSequenceNumber(ctx, retriever, "yaba", "5.0")
	require.Equal(t, ErrInvalidSettingsFile, err)
}

func Test_getCurrentSequenceNumberFound(t *testing.T) {
	retriever := mockSequenceNumberRetriever{returnSeqNo: 42, returnError: nil}
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	seqNo, err := getCurrentSequenceNumber(ctx, retriever, "yaba", "5.0")
	require.NoError(t, err, "getCurrentSequenceNumber failed")
	require.Equal(t, uint(42), seqNo)
}

func Test_findSeqNoFolderDoesntExist(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	seqNo, err := findSeqNum(ctx, "./yabamonster")
	require.Equal(t, uint(0), seqNo)
	require.Error(t, err)
}

func Test_findSeqNoFilesInDifferentOrder(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "5")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "4")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "3")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "2")

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.NoError(t, err, "findSeqNum failed")
	require.Equal(t, uint(2), seqNo)
}

func Test_findSeqNoNoFilesInFolder(t *testing.T) {
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.Equal(t, ErrNoSettingsFiles, err)
	require.Equal(t, uint(0), seqNo)
}

func Test_findSeqNoInvalidFileName(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "0")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "yaba")

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.Equal(t, ErrInvalidSettingsFileName, err)
	require.Equal(t, uint(0), seqNo)
}

func Test_findSeqNoFilesSameTimestamp(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	timeStamp := time.Now()
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "3", timeStamp)
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "2", timeStamp)
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "1", timeStamp)
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "0", timeStamp)

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.NoError(t, err, "findSeqNum failed")
	require.Equal(t, uint(3), seqNo)
}

func Test_findSeqNoFilesSameTimestampOneInvalid(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	timeStamp := time.Now()
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "3", timeStamp)
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "2", timeStamp)
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "yaba", timeStamp)
	writeSequenceNumberFileTs(t, sequenceNumberTestFolder, "0", timeStamp)

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.Equal(t, ErrInvalidSettingsFileName, err)
	require.Equal(t, uint(0), seqNo)
}

func Test_findSeqNoVeryDifferentNumbers(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "0")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "117")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "2942")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "35749")

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.NoError(t, err, "findSeqNum failed")
	require.Equal(t, uint(35749), seqNo)
}

func Test_findSeqNoOnlyOneFile(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "157")

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.NoError(t, err, "findSeqNum failed")
	require.Equal(t, uint(157), seqNo)
}

func Test_findSeqNoNormalExecution(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cleanupTestDirectory(t, sequenceNumberTestFolder)
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "0")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "1")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "2")
	writeSequenceNumberFile(t, sequenceNumberTestFolder, "3")

	seqNo, err := findSeqNum(ctx, sequenceNumberTestFolder)
	require.NoError(t, err, "findSeqNum failed")
	require.Equal(t, uint(3), seqNo)
}

func writeSequenceNumberFileTs(t *testing.T, testDirectory string, name string, timeStamp time.Time) {
	fullPath := writeSequenceNumberFile(t, testDirectory, name)
	err := os.Chtimes(fullPath, timeStamp, timeStamp)
	require.NoError(t, err, "Chtimes failed")
}

func writeSequenceNumberFile(t *testing.T, testDirectory string, name string) string {
	fullName := name + ".settings"
	fullPath := path.Join(testDirectory, fullName)
	data := []byte("this doesn't matter")
	err := ioutil.WriteFile(fullPath, data, 0644)
	require.NoError(t, err, "WriteFile failed")

	return fullPath
}

func cleanupTestDirectory(t *testing.T, testDirectory string) {
	// Create the directory if it doesn't already exist
	_ = os.Mkdir(testDirectory, os.ModePerm)

	// Open the directory and read all its files.
	dirRead, err := os.Open(testDirectory)
	require.NoError(t, err, "os.Open failed")
	dirFiles, err := dirRead.Readdir(0)
	require.NoError(t, err, "Readdir failed")

	// Loop over the directory's files.
	for index := range dirFiles {
		fileToDelete := dirFiles[index]
		fullPath := path.Join(testDirectory, fileToDelete.Name())
		err = os.Remove(fullPath)
		require.NoError(t, err, "os.Remove failed")
	}
}
