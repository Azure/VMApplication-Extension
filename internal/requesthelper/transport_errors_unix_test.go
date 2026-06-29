//go:build !windows

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package requesthelper_test

import "syscall"

func platformConnectionResetError() error {
	return syscall.ECONNRESET
}

func platformConnectionResetErrorText() string {
	return "connection reset by peer"
}