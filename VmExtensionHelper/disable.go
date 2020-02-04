package vmextensionhelper

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/go-kit/kit/log"
)

const disabledFileName = "disable"

var (
	disableDependency disableDependencies = disableDependencyImpl{}
)

type disableDependencies interface {
	writeFile(string, []byte, os.FileMode) error
	remove(name string) error
}

type disableDependencyImpl struct{}

func (disableDependencyImpl) writeFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

func (disableDependencyImpl) remove(name string) error {
	return os.Remove(name)
}

func disable(ctx log.Logger, ext *VMExtension) (string, error) {
	ctx.Log("event", "disable")

	if ext.exec.supportsDisable {
		ctx.Log("event", "Disabling extension")
		if isDisabled(ctx, ext) {
			ctx.Log("message", "Extension is already disabled")
		} else {
			err := setDisabled(ctx, ext, true)
			if err != nil {
				return "", err
			}
		}
	}

	// Call the callback if we have one
	if ext.exec.disableCallback != nil {
		err := ext.exec.disableCallback(ctx, ext)
		if err != nil {
			ctx.Log("message", "Disable failed", "error", err)
			return "", err
		}
	}

	return "", nil
}

func isDisabled(ctx log.Logger, ext *VMExtension) bool {
	// We are disabled if the disabled file exists in the config folder
	disabledFile := path.Join(ext.HandlerEnv.ConfigFolder, disabledFileName)
	exists, _ := doesFileExist(disabledFile)
	return exists
}

func setDisabled(ctx log.Logger, ext *VMExtension, disabled bool) error {
	disabledFile := path.Join(ext.HandlerEnv.ConfigFolder, disabledFileName)
	exists, _ := doesFileExist(disabledFile)
	if exists != disabled {
		if disabled {
			// Create the file
			ctx.Log("Event", "Disabling extension")
			b := []byte("1")
			err := disableDependency.writeFile(disabledFile, b, 0)
			if err != nil {
				ctx.Log("message", "Could not disable the extension", "error", err)
				return err
			}

			ctx.Log("Event", "Disabled extension")
		} else {
			// Remove the file
			ctx.Log("Event", "Un-disabling extension")
			err := disableDependency.remove(disabledFile)
			if err != nil {
				ctx.Log("message", "Could not re-enable the extension", "error", err)
				return err
			}

			ctx.Log("Event", "Re-enabled extension")
		}
	}

	return nil
}
