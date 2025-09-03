package requesthelper

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Azure/azure-extension-platform/pkg/logging"
	"github.com/pkg/errors"
)

const (
	// Predefined token directories for Arc agent
	arcTokensLinuxDir   = "/var/opt/azcmagent/tokens"
	arcTokensWindowsDir = `${PROGRAMDATA}\AzureConnectedMachineAgent\Tokens`

	// Required file extension for challenge files
	arcChallengeFileExtension = ".key"

	WWWAuthenticateHeader = "WWW-Authenticate"
)

type ArcAuthHandler struct {
	baseManager *RequestManager
}

func NewArcAuthHandler(rm *RequestManager) *ArcAuthHandler {
	return &ArcAuthHandler{
		baseManager: rm,
	}
}

func (a *ArcAuthHandler) MakeArcRequest(el *logging.ExtensionLogger) (*http.Response, error) {
	resp, err := a.baseManager.MakeRequest()
	if err == nil {
		return resp, nil
	}

	// Check if this is a 401 challenge from Arc agent
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		defer resp.Body.Close()

		// Look for WWW-Authenticate header with challenge file path
		wwwAuth := resp.Header.Get(WWWAuthenticateHeader)
		if wwwAuth != "" {
			el.Info("Arc agent challenge received, attempting challenge-response authentication")
			return a.handleArcChallenge(el, wwwAuth)
		}
	}

	// Not an Arc challenge, return the original error
	return resp, err
}

func (a *ArcAuthHandler) handleArcChallenge(el *logging.ExtensionLogger, wwwAuth string) (*http.Response, error) {
	challengePath, err := parseArcAuthHeader(wwwAuth)
	if err != nil {
		el.Error("Failed to parse Arc challenge: %v", err)
		return nil, errors.Wrap(err, "failed to parse Arc authentication challenge")
	}

	if err := validateArcChallengePath(el, challengePath); err != nil {
		el.Error("Arc challenge validation failed: %v", err)
		return nil, errors.Wrap(err, "Arc challenge validation failed")
	}

	token, err := readArcToken(el, challengePath)
	if err != nil {
		el.Error("Failed to read Arc token: %v", err)
		return nil, errors.Wrap(err, "failed to read Arc authentication token")
	}

	return a.makeRequestWithToken(el, token)
}

func parseArcAuthHeader(wwwAuth string) (string, error) {
	// The WWW-Authenticate header should contain the file path
	// Expected format might be: "Basic realm=/path/to/token.key"
	if after, ok := strings.CutPrefix(wwwAuth, "Basic realm="); ok {
		return after, nil
	}

	return "", errors.Errorf("unrecognized WWW-Authenticate format: %s", wwwAuth)
}

func readArcToken(el *logging.ExtensionLogger, challengePath string) (string, error) {
	el.Info("Reading Arc token from: %s", challengePath)

	tokenBytes, err := os.ReadFile(challengePath)
	if err != nil {
		if os.IsNotExist(err) {
			el.Error("Arc token file not found: %s", challengePath)
			return "", errors.Errorf("Arc token file not found: %s", challengePath)
		}
		el.Error("Failed to read Arc token file %s: %v", challengePath, err)
		return "", errors.Wrapf(err, "failed to read Arc token file: %s", challengePath)
	}

	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		el.Error("Arc token file is empty: %s", challengePath)
		return "", errors.Errorf("Arc token file is empty: %s", challengePath)
	}

	el.Info("Successfully read Arc token from: %s", challengePath)
	return token, nil
}

func (a *ArcAuthHandler) makeRequestWithToken(el *logging.ExtensionLogger, token string) (*http.Response, error) {
	req, err := a.baseManager.requestFactory.GetRequest()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create authenticated request")
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", token))

	el.Info("Making authenticated request to Arc agent")

	resp, err := a.baseManager.httpClient.Do(req)
	if err != nil {
		el.Error("Authenticated Arc request failed: %v", err)
		return resp, errors.Wrap(err, "authenticated Arc request failed")
	}

	// Check the response status
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		el.Info("Arc authentication successful, status: %d", resp.StatusCode)
		return resp, nil
	}

	el.Error("Arc authentication failed, status: %d", resp.StatusCode)

	return resp, fmt.Errorf("arc authentication failed with status: %d", resp.StatusCode)
}

func validateArcChallengePath(el *logging.ExtensionLogger, challengePath string) error {
	if challengePath == "" {
		el.Error("Empty challenge file path received")
		return errors.New("challenge file path cannot be empty")
	}

	// Validate file extension
	if !strings.HasSuffix(strings.ToLower(challengePath), arcChallengeFileExtension) {
		el.Error("Invalid challenge file extension. Expected %s, got: %s", arcChallengeFileExtension, challengePath)
		return errors.Errorf("challenge file must have %s extension", arcChallengeFileExtension)
	}

	// Get the expected directory based on OS
	expectedDir, err := getExpectedTokenDirectory()
	if err != nil {
		el.Error("Failed to determine expected token directory: %v", err)
		return errors.Wrap(err, "failed to determine expected token directory")
	}

	// Clean and resolve the paths to prevent directory traversal attacks
	cleanChallengePath := filepath.Clean(challengePath)
	cleanExpectedDir := filepath.Clean(expectedDir)

	// Convert to absolute paths for comparison
	absChallengePath, err := filepath.Abs(cleanChallengePath)
	if err != nil {
		el.Error("Failed to resolve challenge path %s: %v", cleanChallengePath, err)
		return errors.Wrapf(err, "failed to resolve challenge path: %s", cleanChallengePath)
	}

	absExpectedDir, err := filepath.Abs(cleanExpectedDir)
	if err != nil {
		el.Error("Failed to resolve expected directory %s: %v", cleanExpectedDir, err)
		return errors.Wrapf(err, "failed to resolve expected directory: %s", cleanExpectedDir)
	}

	// Extract the directory from the challenge file path and compare with expected directory
	challengeDir := filepath.Dir(absChallengePath)
	if challengeDir != absExpectedDir {
		el.Error("Challenge file directory %s does not match expected directory %s", challengeDir, absExpectedDir)
		return errors.Errorf("challenge file directory %s does not match expected directory %s", challengeDir, absExpectedDir)
	}

	return nil
}

// getExpectedTokenDirectory returns the expected token directory based on the current OS
func getExpectedTokenDirectory() (string, error) {
	switch runtime.GOOS {
	case "windows":
		// Expand environment variable for Windows
		return os.ExpandEnv(arcTokensWindowsDir), nil
	case "linux":
		return arcTokensLinuxDir, nil
	default:
		return "", errors.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}
