// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
)

const fiveKilo = 5 * 1024

type VmAppPackageCurrentForStatusCollection []*VmAppPackageCurrentForStatus

type VmAppPackageCurrentForStatus struct {
	ApplicationName string `json:"applicationName"`
	Version         string `json:"version"`
	Result          string `json:"result"`
}

type StatusMessage1 struct {
	CurrentState     VmAppPackageCurrentForStatusCollection `json:"CurrentState"`
	ActionsPerformed actionplan.PackageOperationResults     `json:"ActionsPerformed"`
	Errors           string                                 `json:"Errors"`
}

type StatusMessage2 struct {
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

func getStatusMessage(vmAppCurrentCollection packageregistry.VMAppPackageCurrentCollection, actionPlanExecuteError *actionplan.ExecuteError, result actionplan.IResult) string {
	vmAppCurrentForStatusCollection := getVmAppCurrentForStatus(vmAppCurrentCollection)
	packageOperationResults, ok := result.(*actionplan.PackageOperationResults)
	var statusMessageString string

	if ok {
		var executeErrors = ""
		if err := actionPlanExecuteError.GetErrorIfDeploymentFailed(); err != nil {
			executeErrors = err.Error()
		}
		statusMessageA := StatusMessage1{
			CurrentState:     vmAppCurrentForStatusCollection,
			ActionsPerformed: *packageOperationResults,
			Errors:           executeErrors,
		}

		statusM, err := json.MarshalIndent(statusMessageA, "", " ")
		if err != nil {
			statusMessageString = fmt.Sprintf("%v", statusMessageA)
		} else {
			statusMessageString = string(statusM)
		}

	} else {
		statusMessage := StatusMessage2{
			CurrentState:  vmAppCurrentForStatusCollection,
			CriticalError: result.ToJsonString(),
		}
		statusM, err := json.MarshalIndent(statusMessage, "", " ")
		if err != nil {
			statusMessageString = fmt.Sprintf("%v", statusMessage)
		} else {
			statusMessageString = string(statusM)
		}
	}

	return statusMessageString
}
