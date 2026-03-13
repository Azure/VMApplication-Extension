// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"encoding/json"
	"fmt"
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

// BackupStatusFile renames the current status file so it can be restored later.
// If there is no existing status file, this function returns without error because
// there's nothing to back up.
func BackupStatusFile(statusFolder string, sequenceNumber uint) error {
	current := filepath.Join(statusFolder, fmt.Sprintf("%d.status", sequenceNumber))
	backup := filepath.Join(statusFolder, fmt.Sprintf("%d%s", sequenceNumber, BackupStatusFileSuffix))
	if _, err := os.Stat(current); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(current, backup)
}
