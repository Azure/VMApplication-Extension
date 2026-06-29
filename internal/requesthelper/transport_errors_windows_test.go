//go:build windows

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package requesthelper_test

import "syscall"

func platformConnectionResetError() error {
	return syscall.WSAECONNRESET
}

func platformConnectionResetErrorText() string {
	return "wsarecv: An existing connection was forcibly closed by the remote host"
}