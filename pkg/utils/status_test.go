// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	platformstatus "github.com/Azure/azure-extension-platform/pkg/status"
	"github.com/stretchr/testify/require"
)

func TestStatusParsing(t *testing.T) {
	handlerEnv := handlerenv.HandlerEnvironment{StatusFolder: path.Join(".", "testFiles")}
	statusObj, err := GetStatus(&handlerEnv, 1)
	require.NoError(t, err)
	require.NotNil(t, statusObj)
	require.True(t, strings.EqualFold(string(statusObj.Status), string(platformstatus.StatusTransitioning)))
}

func TestBackupStatusFile(t *testing.T) {
	t.Run("successful backup when status file exists", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()
		statusFile := filepath.Join(tmpDir, "1.status")
		backupFile := filepath.Join(tmpDir, "1"+BackupStatusFileSuffix)

		// Create a status file
		err := os.WriteFile(statusFile, []byte(`[{"status":{"status":"success"}}]`), 0644)
		require.NoError(t, err)

		// Backup the status file
		err = BackupStatusFile(tmpDir, 1)
		require.NoError(t, err)

		// Verify original file no longer exists
		_, err = os.Stat(statusFile)
		require.True(t, os.IsNotExist(err))

		// Verify backup file exists
		_, err = os.Stat(backupFile)
		require.NoError(t, err)
	})

	t.Run("successful backup when status file does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := BackupStatusFile(tmpDir, 999)
		require.NoError(t, err)
	})

	t.Run("error when the status file path is a directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		statusFileAsDir := filepath.Join(tmpDir, "1.status")

		// Create a directory with the same name as the status file
		err := os.Mkdir(statusFileAsDir, 0755)
		require.NoError(t, err)

		// Backup should fail because the path is a directory, not a file
		err = BackupStatusFile(tmpDir, 1)
		require.Error(t, err)
	})
}
