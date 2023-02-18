package utils

import (
	"path"
	"strings"
	"testing"

	"github.com/Azure/azure-extension-platform/pkg/handlerenv"
	platformstatus "github.com/Azure/azure-extension-platform/pkg/status"
	"github.com/stretchr/testify/require"
)

func TestStatusParsing(t *testing.T) {
	handlerEnv := handlerenv.HandlerEnvironment{StatusFolder: path.Join(".", "testFiles")}
	statusType, err := GetStatusType(&handlerEnv, 1)
	require.NoError(t, err)
	require.True(t, strings.EqualFold(string(statusType), string(platformstatus.StatusTransitioning)))
}
