package utils

import (
	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/status"
	"github.com/pkg/errors"
)

type StatusSaveError struct {
	Err error
}

func (statusServerError *StatusSaveError) Error() string {
	return statusServerError.Err.Error()
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
