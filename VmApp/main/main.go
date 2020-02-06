package main

import (
	"VmExtensionHelper/vmextensionhelper"
	"os"

	"github.com/Azure/azure-docker-extension/pkg/vmextension/status"
	"github.com/go-kit/kit/log"
)

var (
	extensionName    = "Microsoft.Azure.Extensions.VMApp"
	extensionVersion = "1.0.0"

	// downloadDir is where we store the downloaded files in the "{downloadDir}/{seqnum}/file"
	// format and the logs as "{downloadDir}/{seqnum}/std(out|err)". Stored under dataDir
	downloadDir = "download"
)

func main() {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout)).With("time", log.DefaultTimestampUTC).With("version", VersionString())
	ii, err := vmextensionhelper.GetInitializationInfo(extensionName, extensionVersion, true, vmAppEnableCallback)

	reportStatus(ctx, hEnv, seqNum, status.StatusSuccess, cmd, msg)
	ctx.Log("event", "end")
}

// Callback indicating the operation is enable and the sequence number has changed
func vmAppEnableCallback(ctx log.Logger, ext *VMExtension) (string, error) {
	return "blah", nil
}
