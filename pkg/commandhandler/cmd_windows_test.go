package commandhandler

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

var cmdHandler = New()


func TestNonExistingCommand(t *testing.T){
	retcode, err := cmdHandler.Execute("command_does_not_exist", workingDir)
	assert.Equal(t, 1, retcode)
	assert.Error(t, err)
}
