package cmd

import (
	"github.com/stretchr/testify/assert"
	"path"
	"testing"
)

var cmdHandler = NewCommandHandler()
var workingDir = path.Join(".", "testdir", "currentWorkingDir")

func TestNonExistingCommand(t *testing.T){
	retcode, err := cmdHandler.Execute("command_does_not_exist", workingDir)
	assert.Equal(t, 1, retcode)
	assert.Error(t, err)
}
