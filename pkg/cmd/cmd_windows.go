package cmd

import (
	"fmt"
	"github.com/pkg/errors"
	"io"
	"os/exec"
	"syscall"
)

func exec2(cmd, workdir string, stdout, stderr io.WriteCloser) (int, error) {
	defer stdout.Close()
	defer stderr.Close()

	c := exec.Command("cmd", "/C", cmd)
	c.Dir = workdir
	c.Stdout = stdout
	c.Stderr = stderr

	err := c.Run()
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			code := status.ExitStatus()
			return code, fmt.Errorf("command terminated with exit status=%d", code)
		}
	}
	return 0, errors.Wrapf(err, "failed to execute command")
}

