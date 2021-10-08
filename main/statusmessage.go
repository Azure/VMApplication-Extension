package main

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/pkg/errors"
)

const fiveKilo = 5 * 1024

var (
	treatFailureAsDeploymentFailureError = errors.New("an app maked as TreatFailureAsDeploymentFailure couldn't be installed or updated")
)

type VmAppPackageCurrentForStatusCollection []*VmAppPackageCurrentForStatus

type VmAppPackageCurrentForStatus struct {
	ApplicationName string `json:"applicationName"`
	Version         string `json:"version"`
	Result          string `json:"result"`
}

type StatusMessageWithPackageOperationResults struct {
	CurrentState     VmAppPackageCurrentForStatusCollection `json:"CurrentState"`
	ActionsPerformed actionplan.PackageOperationResults     `json:"ActionsPerformed"`
}

type StatusMessageWithCriticalError struct {
	CurrentState  VmAppPackageCurrentForStatusCollection `json:"CurrentState"`
	CriticalError string                                 `json:"CriticalError"`
}

func getVmAppCurrentForStatus(vmAppCurrentCollection packageregistry.VMAppPackageCurrentCollection) VmAppPackageCurrentForStatusCollection {
	vmAppCurrentForStatusCollection := make(VmAppPackageCurrentForStatusCollection, 0)
	for _, vmApp := range vmAppCurrentCollection {
		vmAppCurrentForStatusCollection = append(vmAppCurrentForStatusCollection, &VmAppPackageCurrentForStatus{
			ApplicationName: vmApp.ApplicationName,
			Version:         vmApp.Version,
			Result:          vmApp.Result,
		})
	}
	return vmAppCurrentForStatusCollection
}

func getStatusMessageAndError(vmAppCurrentCollection packageregistry.VMAppPackageCurrentCollection, result actionplan.IResult) (string, error) {
	vmAppCurrentForStatusCollection := getVmAppCurrentForStatus(vmAppCurrentCollection)
	packageOperationResults, ok := result.(*actionplan.PackageOperationResults)
	var statusMessageString string

	var errorMessageToReturn error = nil

	if ok {
		statusMessage := StatusMessageWithPackageOperationResults{
			CurrentState:     vmAppCurrentForStatusCollection,
			ActionsPerformed: *packageOperationResults,
		}
		statusM, err := json.MarshalIndent(statusMessage, "", " ")
		if err != nil {
			statusMessageString = fmt.Sprintf("%v", statusMessage)
		} else {
			statusMessageString = string(statusM)
		}
		// figure out if vmApp failure should result in extension error
		var shouldFailEnable bool
		for _, packageOperationResult := range statusMessage.ActionsPerformed {
			if packageOperationResult.TreatFailureAsDeploymentFailure && packageOperationResult.Result != actionplan.Success {
				shouldFailEnable = true
			}
		}
		if shouldFailEnable {
			errorMessageToReturn = treatFailureAsDeploymentFailureError
		}
	} else {
		statusMessage := StatusMessageWithCriticalError{
			CurrentState:  vmAppCurrentForStatusCollection,
			CriticalError: result.ToJsonString(),
		}
		statusM, err := json.MarshalIndent(statusMessage, "", " ")
		if err != nil {
			statusMessageString = fmt.Sprintf("%v", statusMessage)
		} else {
			statusMessageString = string(statusM)
		}
		errorMessageToReturn = errors.New(statusMessage.CriticalError)
	}

	if len(statusMessageString) > fiveKilo {
		statusMessageString = statusMessageString[:fiveKilo]
	}
	return statusMessageString, errorMessageToReturn
}
