package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	"github.com/Azure/azure-extension-platform/pkg/status"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

type StatusSaveError struct {
	Err error
}

func (statusServerError *StatusSaveError) Error() string {
	return statusServerError.Err.Error()
}

func GetStatusType(handlerEnv *handlerenv.HandlerEnvironment, sequenceNumber uint) (status.StatusType, error) {
	fn := fmt.Sprintf("%d.status", sequenceNumber)
	path := filepath.Join(handlerEnv.StatusFolder, fn)
	statusBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	statusReport := make(status.StatusReport, 1)
	err = json.Unmarshal(statusBytes, &statusReport)
	if err != nil {
		return "", err
	}
	return statusReport[0].Status.Status, nil
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

func ReportStatusWithError(handlerEnv *handlerenv.HandlerEnvironment, requestedSequenceNumber uint, operationName string, ewc vmextensionhelper.ErrorWithClarification) error {
	s := status.NewError(operationName, status.ErrorClarification{Code: ewc.ErrorCode, Message: ewc.Err.Error()})
	err := s.Save(handlerEnv.StatusFolder, requestedSequenceNumber)
	if err != nil {
		errorMsg := "failed to save handler status"
		return &StatusSaveError{Err: errors.Wrap(err, errorMsg)}
	}
	return nil
}
