// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/status"
	"github.com/pkg/errors"
)

const BackupStatusFileSuffix = ".lastStableStatus"

type StatusSaveError struct {
	Err error
}

func (statusServerError *StatusSaveError) Error() string {
	return statusServerError.Err.Error()
}

func readStatusFileHelper(path string) (*status.Status, error) {
	statusBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	statusReport := make(status.StatusReport, 1)
	err = json.Unmarshal(statusBytes, &statusReport)
	if err != nil {
		return nil, err
	}
	return &statusReport[0].Status, nil
}

func GetStatus(handlerEnv *handlerenv.HandlerEnvironment, sequenceNumber uint) (*status.Status, error) {
	fn := fmt.Sprintf("%d.status", sequenceNumber)
	path := filepath.Join(handlerEnv.StatusFolder, fn)
	return readStatusFileHelper(path)
}

func GetLastStableStatus(handlerEnv *handlerenv.HandlerEnvironment, sequenceNumber uint) (*status.Status, error) {
	fn := fmt.Sprintf("%d%s", sequenceNumber, BackupStatusFileSuffix)
	path := filepath.Join(handlerEnv.StatusFolder, fn)
	return readStatusFileHelper(path)
}

func ReportStatus(handlerEnv *handlerenv.HandlerEnvironment, requestedSequenceNumber uint, statusType status.StatusType, operationName string, message string) error {
	formattedMessage := status.StatusMsg(operationName, statusType, message)
	s := status.New(statusType, operationName, formattedMessage)
	err := s.Save(handlerEnv.StatusFolder, requestedSequenceNumber)
	if err != nil {
		errorMsg := "failed to save handler status"
		return &StatusSaveError{Err: errors.Wrap(err, errorMsg)}
	}
	return nil
}

// copyFile copies a file from src to dst. If dst already exists, it will be overwritten.
// The file permissions of the destination file will be the same as the source file.
func copyFile(src, dst string) error {
	// Open the source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close() // Get source file info

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Create the destination file with same mode
	destinationFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close()

	// Copy the content
	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Flush file metadata to disk
	err = destinationFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}

// BackupStatusFile renames the current status file so it can be restored later.
// If there is no existing status file, this function returns without error because
// there's nothing to back up.
func BackupStatusFile(statusFolder string, sequenceNumber uint) error {
	current := filepath.Join(statusFolder, fmt.Sprintf("%d.status", sequenceNumber))
	backup := filepath.Join(statusFolder, fmt.Sprintf("%d%s", sequenceNumber, BackupStatusFileSuffix))
	info, err := os.Stat(current)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected a file but found a directory: %s", current)
	}
	return copyFile(current, backup)
}
