package vmextensionhelper

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var (
	updateNormalCallbackCalled bool
	updateErrorCallbackCalled  bool
	mkdirCalled                bool
	removeAllCalled            bool
	statErrorToReturn          error = os.ErrNotExist
	installErrorToReturn       error = os.ErrNotExist
)

type evilInstallDependencies struct{}

func (evilInstallDependencies) mkdirAll(path string, perm os.FileMode) error {
	mkdirCalled = true
	return installErrorToReturn
}

func (evilInstallDependencies) removeAll(path string) error {
	removeAllCalled = true
	return installErrorToReturn
}

func (evilInstallDependencies) stat(name string) (os.FileInfo, error) {
	return nil, statErrorToReturn
}

func Test_updateCallback(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	// Callback succeeds
	updateNormalCallbackCalled = false
	updateErrorCallbackCalled = false
	ext.exec.updateCallback = testUpdateCallbackNormal
	_, err := update(ctx, ext)
	require.NoError(t, err, "Update callback failed")
	require.True(t, updateNormalCallbackCalled)

	// Callback returns an error
	ext.exec.disableCallback = testUpdateCallbackError
	_, err = disable(ctx, ext)
	require.Error(t, err, "oh no. The world is ending, but styling prevents me from using end punctuation or caps")
	require.True(t, updateErrorCallbackCalled)
}

func Test_installAlreadyExists(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	mkdirCalled = false
	statErrorToReturn = nil
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := install(ctx, ext)
	require.NoError(t, err, "install failed")
	require.False(t, mkdirCalled)
}

func Test_installSuccess(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	mkdirCalled = false
	statErrorToReturn = os.ErrNotExist
	installErrorToReturn = nil
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := install(ctx, ext)
	require.NoError(t, err, "install failed")
	require.True(t, mkdirCalled)
}

func Test_installFailToMakeDir(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	mkdirCalled = false
	statErrorToReturn = os.ErrNotExist
	installErrorToReturn = errors.New("something happened")
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := install(ctx, ext)
	require.Error(t, err, installErrorToReturn)
	require.True(t, mkdirCalled)
}

func Test_installFileExistFails(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	mkdirCalled = false
	statErrorToReturn = errors.New("bad permissions")
	installErrorToReturn = os.ErrNotExist
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := install(ctx, ext)
	require.Error(t, err, statErrorToReturn)
	require.False(t, mkdirCalled)
}

func Test_uninstallAlreadyGone(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	removeAllCalled = false
	statErrorToReturn = os.ErrNotExist
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := uninstall(ctx, ext)
	require.NoError(t, err, "uninstall failed")
	require.False(t, removeAllCalled)
}

func Test_uninstallSuccess(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	removeAllCalled = false
	statErrorToReturn = nil
	installErrorToReturn = nil
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := uninstall(ctx, ext)
	require.NoError(t, err, "uninstall failed")
	require.True(t, removeAllCalled)
}

func Test_uninstallFailToRemoveDir(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	removeAllCalled = false
	statErrorToReturn = nil
	installErrorToReturn = errors.New("something happened")
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := uninstall(ctx, ext)
	require.Error(t, err, installErrorToReturn)
	require.True(t, removeAllCalled)
}

func Test_uninstallFileExistFails(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtensionForDisable()

	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	removeAllCalled = false
	statErrorToReturn = errors.New("bad permissions")
	installErrorToReturn = os.ErrNotExist
	installDependency = evilInstallDependencies{}
	defer resetDependencies()

	_, err := uninstall(ctx, ext)
	require.Error(t, err, statErrorToReturn)
	require.False(t, removeAllCalled)
}

func resetInstallDependencies() {
	installDependency = installDependencyImpl{}
}

func testUpdateCallbackNormal(ctx log.Logger, ext *VMExtension) error {
	updateNormalCallbackCalled = true
	return nil
}

func testUpdateCallbackError(ctx log.Logger, ext *VMExtension) error {
	updateErrorCallbackCalled = true
	return fmt.Errorf("oh no. The world is ending, but styling prevents me from using end punctuation or caps")
}
