package lockedfile

import (
	"path"
	"testing"
	"time"
)

const testdir = ".testdir"

var testFilePath = path.Combine(testdir, "temp.lockedfile")

func TestLockFileCanBeWrittenAndRead(t *testing.T){
	var lf ILockedFile
	lf, err := New(testFilePath, time.Second)
}