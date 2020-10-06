package commandhandler

import (
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"testing"
)

var workingDir = path.Join(".", "testdir", "currentWorkingDir")

func cleanupTest(){
	os.RemoveAll(workingDir)
}

func TestEchoCommand(t *testing.T){
	defer cleanupTest()
	cmd := New()
	retCode, err := cmd.Execute("echo 1 2 3 4", workingDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, retCode, "return code should be 0")
}
