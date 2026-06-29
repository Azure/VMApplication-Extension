//go:build !windows

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package requesthelper

import (
	"errors"
	"syscall"
)

func isPlatformConnectionResetError(err error) bool {
	return errors.Is(err, syscall.ECONNRESET)
}