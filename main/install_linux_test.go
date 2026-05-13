// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_vmAppInstallCallback_restoresDataFolderFromBackupWhenDataFolderMissing(t *testing.T) {
	ext := createTestVMExtension(t, nil)

	root := t.TempDir()
	dataFolder := filepath.Join(root, "data")
	ext.HandlerEnv.DataFolder = dataFolder
	backupDir := getDataFolderBackupPath(ext)

	backupFilePath := filepath.Join(backupDir, "downloadedPackages", "appA", "payload.txt")
	err := os.MkdirAll(filepath.Dir(backupFilePath), 0755)
	require.NoError(t, err)
	expectedContent := []byte("from backup")
	err = os.WriteFile(backupFilePath, expectedContent, 0644)
	require.NoError(t, err)

	err = vmAppInstallCallback(ext)
	require.NoError(t, err)

	_, err = os.Stat(backupDir)
	require.True(t, os.IsNotExist(err), "backup directory should be renamed away")

	restoredContent, err := os.ReadFile(filepath.Join(dataFolder, "downloadedPackages", "appA", "payload.txt"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(expectedContent, restoredContent), "restored DataFolder should contain original backup content")
}

func Test_vmAppInstallCallback_mergesBackupIntoExistingDataFolderAndPrefersExistingEntries(t *testing.T) {
	ext := createTestVMExtension(t, nil)

	root := t.TempDir()
	dataFolder := filepath.Join(root, "data")
	ext.HandlerEnv.DataFolder = dataFolder
	backupDir := getDataFolderBackupPath(ext)

	// Existing DataFolder content should be preserved on name collisions.
	existingCollisionPath := filepath.Join(dataFolder, "appA", "payload.txt")
	err := os.MkdirAll(filepath.Dir(existingCollisionPath), 0755)
	require.NoError(t, err)
	existingContent := []byte("existing datafolder content")
	err = os.WriteFile(existingCollisionPath, existingContent, 0644)
	require.NoError(t, err)

	// Backup has a colliding file and a non-colliding file.
	backupCollisionPath := filepath.Join(backupDir, "appA", "payload.txt")
	err = os.MkdirAll(filepath.Dir(backupCollisionPath), 0755)
	require.NoError(t, err)
	err = os.WriteFile(backupCollisionPath, []byte("backup content that should be ignored"), 0644)
	require.NoError(t, err)

	backupOnlyPath := filepath.Join(backupDir, "appB", "pkg.txt")
	err = os.MkdirAll(filepath.Dir(backupOnlyPath), 0755)
	require.NoError(t, err)
	backupOnlyContent := []byte("backup-only content")
	err = os.WriteFile(backupOnlyPath, backupOnlyContent, 0644)
	require.NoError(t, err)

	err = vmAppInstallCallback(ext)
	require.NoError(t, err)

	collisionContentAfterInstall, err := os.ReadFile(existingCollisionPath)
	require.NoError(t, err)
	require.True(t, bytes.Equal(existingContent, collisionContentAfterInstall), "existing DataFolder entry should win on collision")

	copiedFromBackup, err := os.ReadFile(filepath.Join(dataFolder, "appB", "pkg.txt"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(backupOnlyContent, copiedFromBackup), "non-colliding backup entry should be moved into DataFolder")

	_, err = os.Stat(backupDir)
	require.True(t, os.IsNotExist(err), "backup directory should be cleaned up after merge")
}
