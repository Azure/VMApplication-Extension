// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"encoding/json"

	"github.com/Azure/VMApplication-Extension/internal/actionplan"
	"github.com/Azure/VMApplication-Extension/internal/packageregistry"
	"github.com/stretchr/testify/assert"

	"math/rand"
	"strings"
	"testing"
)

var vmAppCurrentCollection = packageregistry.VMAppPackageCurrentCollection{
	&packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app1",
		Version:          "0.1.1",
		OngoingOperation: packageregistry.NoAction,
		Result:           "install succeeded",
	},
	&packageregistry.VMAppPackageCurrent{
		ApplicationName:  "app2",
		Version:          "1.1.0",
		OngoingOperation: packageregistry.Failed,
		Result:           "update failed",
	},
}

type criticalError string

func (c criticalError) ToJsonString() string {
	return string(c)
}

var executeError = actionplan.ExecuteError{}

func TestGetStatusMessage01(t *testing.T) {
	actionsPerformed := actionplan.PackageOperationResults{
		actionplan.PackageOperationResult{
			PackageName: "app1",
			AppVersion:  "0.1.1",
			Operation:   "Install",
			Result:      "install succeeded",
		},
		actionplan.PackageOperationResult{
			PackageName: "app1",
			AppVersion:  "0.1.1",
			Operation:   "GetLogs",
			Result:      "Success",
		},
	}

	statusMessage := getStatusMessage(vmAppCurrentCollection, &executeError, &actionsPerformed)
	statusMessage1 := new(StatusMessage1)
	err := json.Unmarshal([]byte(statusMessage), statusMessage1)
	assert.NoError(t, err)
	assertCollectionsMatch(t, vmAppCurrentCollection, statusMessage1.CurrentState)
	assert.EqualValues(t, actionsPerformed, statusMessage1.ActionsPerformed)
}

func TestGetStatusMessage02(t *testing.T) {
	var ce criticalError = "critical error"
	statusMessage := getStatusMessage(vmAppCurrentCollection, &executeError, &ce)
	statusMessage2 := new(StatusMessage2)
	err := json.Unmarshal([]byte(statusMessage), statusMessage2)
	assert.NoError(t, err)
	assertCollectionsMatch(t, vmAppCurrentCollection, statusMessage2.CurrentState)
	assert.EqualValues(t, ce, statusMessage2.CriticalError)
}

func assertCollectionsMatch(t *testing.T, vmAppCurrentCollection packageregistry.VMAppPackageCurrentCollection, vmAppCurrentForStatusCollection VmAppPackageCurrentForStatusCollection) {
	assert.Equal(t, len(vmAppCurrentCollection), len(vmAppCurrentForStatusCollection), "lengths should match")
	for i, vmAppCurrent := range vmAppCurrentCollection {
		vmAppStatus := vmAppCurrentForStatusCollection[i]
		assert.Equal(t, vmAppCurrent.ApplicationName, vmAppStatus.ApplicationName)
		assert.Equal(t, vmAppCurrent.Version, vmAppStatus.Version)
		assert.Equal(t, vmAppCurrent.Result, vmAppStatus.Result)
	}
}

func generateRandomStringOfLength(length int) string {
	charset := "abcdABCD1234"
	charsetLength := len(charset)
	sb := strings.Builder{}
	for i := 0; i < length; i++ {
		randChar := charset[rand.Intn(charsetLength)]
		sb.WriteString(string(randChar))
	}
	return sb.String()
}
