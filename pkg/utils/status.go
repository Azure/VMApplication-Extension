package utils

import (
	"github.com/Azure/azure-extension-platform/pkg/status"
	vmextensionhelper "github.com/Azure/azure-extension-platform/vmextension"
	"github.com/pkg/errors"
)

func ReportStatus(requestedSequenceNumber uint, statusType status.StatusType, operationName string, message string) error {
	formattedMessage := status.StatusMsg(operationName, statusType, message)
	s := status.New(statusType, operationName, formattedMessage)
	err = s.Save(vmextension.HandlerEnv.StatusFolder, requestedSequenceNumber)
	if err != nil {
		errorMsg := "failed to save handler status"
		vmextension.ExtensionEvents.LogCriticalEvent("Write extension status", errorMsg)
		vmextension.ExtensionLogger.Error(errorMsg)
		return errors.Wrap(err, errorMsg)
	}
	return nil
}