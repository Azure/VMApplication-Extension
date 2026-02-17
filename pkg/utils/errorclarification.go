// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

const (
	// System errors are less than 0
	FileSystem_CannotObtainLock          = -50
	FileSystem_CouldNotRemoveFile        = -51
	FileSystem_CouldNotOpenDownloadFile  = -52
	FileSystem_CouldNotRetrieveFileStats = -53
	FileSystem_CouldNotSeekEOF           = -54
	FileSystem_CouldNotCopyResponseData  = -55

	Infra_CouldNotGetSettings                  = -40
	Infra_CouldNotDeserializeProtectedSettings = -41

	DataFormat_MissingApplicationName = -60
	DataFormat_CouldNotParseHostGAUri = -61

	HGAP_FailedToAddDefaultPort         = -70
	HGAP_InvalidAppInfo                 = -71
	HGAP_PackageDownloadFailed          = -72
	HGAP_UnexpectedPackageStatusCode    = -73
	HGAP_FailedToCompletelyDownloadFile = -74

	Metadata_RequestFailure         = -80
	Metadata_CouldNotDecodeResponse = -81

	PackageRegistry_DoesNotExist           = -90
	PackageRegistry_CouldNotUnmarshal      = -91
	PackageRegistry_CouldNotAccess         = -92
	PackageRegistry_DuplicateName          = -93
	PackageRegistry_RenameFailed           = -94
	PackageRegistry_CouldNotReadCollection = -95
	PackageRegistry_CouldNotRemoveBackup   = -96
	PackageRegistry_CouldNotWrite          = -97
	PackageRegistry_UnknownAction          = -98

	Execute_FailedToCreateDownloadDir = -110

	// User errors are greater than 0
	Execute_RebootsExceeded     = 10
	Execute_RemovedFromRegistry = 11
)
