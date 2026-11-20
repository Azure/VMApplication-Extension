// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package actionplan

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/stretchr/testify/require"
)

type lateSignalRegistry struct {
	packageregistry.IPackageRegistry
	signal syscall.Signal
	writes int
}

func (registry *lateSignalRegistry) WriteToDisk(packages packageregistry.CurrentPackageRegistry) error {
	registry.writes++
	if registry.writes == 2 {
		if err := syscall.Kill(os.Getpid(), registry.signal); err != nil {
			return err
		}
		// Wait for default signal handling; the parent enforces a timeout.
		time.Sleep(time.Hour)
	}
	return registry.IPackageRegistry.WriteToDisk(packages)
}

func TestCommandCompletionStopsSignalNotifications(t *testing.T) {
	const childVariable = "VMAPP_LATE_SIGNAL_TEST"
	cases := []struct {
		name    string
		retCode int
		signal  syscall.Signal
	}{
		{"success SIGTERM", 0, syscall.SIGTERM},
		{"failure SIGTERM", 1, syscall.SIGTERM},
		{"signal failure SIGTERM", -1, syscall.SIGTERM},
		{"success SIGINT", 0, syscall.SIGINT},
		{"failure SIGINT", 1, syscall.SIGINT},
		{"signal failure SIGINT", -1, syscall.SIGINT},
	}
	if child := os.Getenv(childVariable); child != "" {
		index, err := strconv.Atoi(child)
		require.NoError(t, err)
		require.GreaterOrEqual(t, index, 0)
		require.Less(t, index, len(cases))
		test := cases[index]
		initTest(t)
		defer cleanTest()
		previousCheck := systemShutdownInProgress
		defer func() { systemShutdownInProgress = previousCheck }()
		systemShutdownInProgress = func() (bool, error) { return false, nil }
		registry := &lateSignalRegistry{IPackageRegistry: packageRegistry, signal: test.signal}
		command := NewCommandHandlerMock(func(string, string) (int, error) {
			if test.retCode != 0 {
				return test.retCode, errors.New("command failed")
			}
			return 0, nil
		})
		_ = actionPlan.executeHelper(registry, command, make(packageregistry.CurrentPackageRegistry),
			&action{vmAppPackage: vmAppPackageCurrent, actionToPerform: packageregistry.Install}, extensionEventManager)
		t.Fatal("late signal did not terminate the process during the final registry write")
	}
	for index, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandCompletionStopsSignalNotifications$")
			command.Dir = t.TempDir()
			command.Env = append(os.Environ(), childVariable+"="+strconv.Itoa(index))
			output, err := command.CombinedOutput()
			require.NoError(t, ctx.Err(), "late signal was swallowed: %s", output)
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr, "child output: %s", output)
			waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
			require.True(t, ok)
			require.True(t, waitStatus.Signaled(), "child did not exit from a signal: %s", output)
			require.Equal(t, test.signal, waitStatus.Signal())
		})
	}
}
