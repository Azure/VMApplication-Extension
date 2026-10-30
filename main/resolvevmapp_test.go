// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"errors"
	"testing"
	"time"

	"github.com/Azure/VMApplication-Extension/internal/extdeserialization"
	"github.com/Azure/VMApplication-Extension/internal/hostgacommunicator"
	"github.com/Azure/VMApplication-Extension/internal/requesthelper"
	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/stretchr/testify/require"
)

type staleMetadataCommunicator struct {
	NoopHostGaCommunicator
	versions []string
	calls    int
}

func (communicator *staleMetadataCommunicator) GetVMAppInfo(el *logging.ExtensionLogger, appName string, version string) (*hostgacommunicator.VMAppMetadata, error) {
	metadata := &hostgacommunicator.VMAppMetadata{
		ApplicationName: appName,
		Version:         communicator.versions[communicator.calls],
	}
	communicator.calls++
	return metadata, nil
}

type fallbackMetadataCommunicator struct {
	NoopHostGaCommunicator
	getVMAppInfo func(string, string) (*hostgacommunicator.VMAppMetadata, error)
}

func (communicator *fallbackMetadataCommunicator) GetVMAppInfo(el *logging.ExtensionLogger, appName string, version string) (*hostgacommunicator.VMAppMetadata, error) {
	return communicator.getVMAppInfo(appName, version)
}

func Test_getVMAppIncomingCollection_RetriesVersionMismatch(t *testing.T) {
	originalSleep := requesthelper.ActualSleep
	requesthelper.ActualSleep = func(time.Duration) {}
	t.Cleanup(func() { requesthelper.ActualSleep = originalSleep })

	communicator := &staleMetadataCommunicator{versions: []string{"1.0.0", "2.0.0"}}
	settings := extdeserialization.VmAppProtectedSettings{
		&extdeserialization.VmAppSetting{
			ApplicationName: "iggy",
			Version:         "2.0.0",
		},
	}

	incoming, err := getVMAppIncomingCollection(settings, communicator, nopLog())
	require.NoError(t, err)
	require.Equal(t, 2, communicator.calls)
	require.Equal(t, "2.0.0", incoming[0].Version)
}

func Test_getVMAppIncomingCollection_AcceptsResolvedVersionWhenVersionUnspecified(t *testing.T) {
	communicator := &staleMetadataCommunicator{versions: []string{"2.0.0"}}
	settings := extdeserialization.VmAppProtectedSettings{
		&extdeserialization.VmAppSetting{
			ApplicationName: "iggy",
		},
	}

	incoming, err := getVMAppIncomingCollection(settings, communicator, nopLog())
	require.NoError(t, err)
	require.Equal(t, 1, communicator.calls)
	require.Equal(t, "2.0.0", incoming[0].Version)
}

func Test_getVMAppIncomingCollection_Fallback(t *testing.T) {
	originalSleep := requesthelper.ActualSleep
	requesthelper.ActualSleep = func(time.Duration) {}
	t.Cleanup(func() { requesthelper.ActualSleep = originalSleep })

	primaryErr := errors.New("versioned metadata request failed")
	fallbackErr := errors.New("unversioned metadata request failed")
	for _, tt := range []struct {
		name            string
		primaryVersion  string
		primaryErr      error
		fallbackVersion string
		fallbackErr     error
		primaryCalls    int
		wantFallback    bool
		wantErr         error
		wantInvalid     bool
	}{
		{name: "primary success skips fallback", primaryVersion: "2.0.0", primaryCalls: 1},
		{name: "request failure falls back to different version", primaryErr: primaryErr, fallbackVersion: "1.0.0", primaryCalls: 1, wantFallback: true},
		{name: "exhausted version mismatches fall back", primaryVersion: "1.5.0", fallbackVersion: "1.0.0", primaryCalls: 7, wantFallback: true},
		{name: "empty primary version falls back", fallbackVersion: "1.0.0", primaryCalls: 1, wantFallback: true},
		{name: "fallback request error is returned", primaryErr: primaryErr, fallbackErr: fallbackErr, primaryCalls: 1, wantFallback: true, wantErr: fallbackErr},
		{name: "empty fallback version is rejected", primaryErr: primaryErr, primaryCalls: 1, wantFallback: true, wantInvalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			primaryMetadata := &hostgacommunicator.VMAppMetadata{
				ApplicationName: "iggy",
				Version:         tt.primaryVersion,
			}
			fallbackMetadata := &hostgacommunicator.VMAppMetadata{
				ApplicationName: "iggy",
				Version:         tt.fallbackVersion,
				InstallCommand:  "install fallback",
				RemoveCommand:   "remove fallback",
				UpdateCommand:   "update fallback",
				ConfigExists:    true,
				PackageFileName: "fallback.pkg",
				ConfigFileName:  "fallback.config",
			}
			var requestedVersions []string
			communicator := &fallbackMetadataCommunicator{
				getVMAppInfo: func(appName, version string) (*hostgacommunicator.VMAppMetadata, error) {
					require.Equal(t, "iggy", appName)
					requestedVersions = append(requestedVersions, version)
					require.LessOrEqual(t, len(requestedVersions), tt.primaryCalls+1)
					if version == "" {
						if tt.fallbackErr != nil {
							return nil, tt.fallbackErr
						}
						return fallbackMetadata, nil
					}
					if tt.primaryErr != nil {
						return nil, tt.primaryErr
					}
					return primaryMetadata, nil
				},
			}
			settings := extdeserialization.VmAppProtectedSettings{
				&extdeserialization.VmAppSetting{ApplicationName: "iggy", Version: "2.0.0"},
			}

			incoming, err := getVMAppIncomingCollection(settings, communicator, nopLog())

			wantVersions := make([]string, tt.primaryCalls)
			for i := range wantVersions {
				wantVersions[i] = "2.0.0"
			}
			if tt.wantFallback {
				wantVersions = append(wantVersions, "")
			}
			require.Equal(t, wantVersions, requestedVersions)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Empty(t, incoming)
				return
			}
			if tt.wantInvalid {
				require.EqualError(t, err, "HostGA did not return a valid vmAppInfo")
				require.Empty(t, incoming)
				return
			}
			require.NoError(t, err)
			require.Len(t, incoming, 1)
			expectedMetadata := primaryMetadata
			if tt.wantFallback {
				expectedMetadata = fallbackMetadata
			}
			require.Equal(t, expectedMetadata.Version, incoming[0].Version)
			require.Equal(t, expectedMetadata.InstallCommand, incoming[0].InstallCommand)
			require.Equal(t, expectedMetadata.RemoveCommand, incoming[0].RemoveCommand)
			require.Equal(t, expectedMetadata.UpdateCommand, incoming[0].UpdateCommand)
			require.Equal(t, expectedMetadata.ConfigExists, incoming[0].ConfigExists)
			require.Equal(t, expectedMetadata.PackageFileName, incoming[0].PackageFileName)
			require.Equal(t, expectedMetadata.ConfigFileName, incoming[0].ConfigFileName)
		})
	}
}
