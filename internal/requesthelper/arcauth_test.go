package requesthelper

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-extension-platform/pkg/logging"
)

// mockRequestFactory for testing
type mockRequestFactory struct {
	url string
}

func (m *mockRequestFactory) GetRequest() (*http.Request, error) {
	return http.NewRequest("GET", m.url, nil)
}

func TestArcAuthHandler_MakeArcRequest_Success(t *testing.T) {
	// Create a test server that returns 200 OK on the first request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	factory := &mockRequestFactory{url: server.URL}
	requestManager := GetRequestManager(factory, 5*time.Second)
	arcHandler := NewArcAuthHandler(requestManager)

	logger := logging.New(nil)

	resp, err := arcHandler.MakeArcRequest(logger)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got: %d", resp.StatusCode)
	}
}

func TestArcAuthHandler_MakeArcRequest_ChallengeResponse_NoValidation(t *testing.T) {
	// This test bypasses path validation to test the core challenge-response logic
	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "test.key")
	tokenContent := "test-token-12345"

	err := os.WriteFile(tokenFile, []byte(tokenContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test token file: %v", err)
	}

	requestCount := 0

	// Create a test server that returns 401 on first request with challenge, 200 on second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount == 1 {
			// First request - return challenge
			w.Header().Set("WWW-Authenticate", "Basic realm="+tokenFile)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("challenge"))
		} else {
			// Second request - check for auth header
			authHeader := r.Header.Get("Authorization")
			expectedAuth := "Basic " + tokenContent

			if authHeader != expectedAuth {
				t.Errorf("Expected Authorization header '%s', got '%s'", expectedAuth, authHeader)
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("authenticated success"))
		}
	}))
	defer server.Close()

	factory := &mockRequestFactory{url: server.URL}
	requestManager := GetRequestManager(factory, 5*time.Second)

	logger := logging.New(nil)

	// Mock the validation function by directly calling the token flow
	// First request should fail with 401
	resp, err := requestManager.MakeRequest()
	if err == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized on first request")
	}
	resp.Body.Close()

	wwwAuth := resp.Header.Get("WWW-Authenticate")
	challengePath, err := parseArcAuthHeader(wwwAuth)
	if err != nil {
		t.Fatalf("Failed to parse challenge: %v", err)
	}

	token, err := readArcToken(logger, challengePath)
	if err != nil {
		t.Fatalf("Failed to read token: %v", err)
	}

	req, err := factory.GetRequest()
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Basic "+token)

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed authenticated request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got: %d", resp.StatusCode)
	}
}

func TestParseArcChallenge(t *testing.T) {
	tests := []struct {
		name     string
		wwwAuth  string
		expected string
		hasError bool
	}{
		{
			name:     "Basic auth",
			wwwAuth:  "Basic realm=/path/to/token.key",
			expected: "/path/to/token.key",
			hasError: false,
		},
		{
			name:     "file= format",
			wwwAuth:  "file=/path/to/token.key",
			expected: "",
			hasError: true,
		},
		{
			name:     "Direct path with .key",
			wwwAuth:  "/path/to/token.key",
			expected: "",
			hasError: true,
		},
		{
			name:     "Invalid format",
			wwwAuth:  "invalid-format",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseArcAuthHeader(tt.wwwAuth)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected '%s', got '%s'", tt.expected, result)
				}
			}
		})
	}
}

func TestReadArcToken(t *testing.T) {
	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "test.key")
	tokenContent := "test-token-12345"

	err := os.WriteFile(tokenFile, []byte(tokenContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test token file: %v", err)
	}

	// Create logger
	logger := logging.New(nil)

	// Test reading the token
	token, err := readArcToken(logger, tokenFile)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if token != tokenContent {
		t.Fatalf("Expected token '%s', got '%s'", tokenContent, token)
	}
}

func TestReadArcToken_FileNotFound(t *testing.T) {
	// Create logger
	logger := logging.New(nil)

	// Test reading a non-existent file
	_, err := readArcToken(logger, "/non/existent/file.key")
	if err == nil {
		t.Fatalf("Expected error for non-existent file, got none")
	}
}

func TestValidateArcChallengePath(t *testing.T) {
	// Create logger
	logger := logging.New(nil)

	tests := []struct {
		name          string
		challengePath string
		setupFunc     func(t *testing.T) (string, func())
		shouldError   bool
		errorContains string
	}{
		{
			name:          "Empty path",
			challengePath: "",
			setupFunc:     func(t *testing.T) (string, func()) { return "", func() {} },
			shouldError:   true,
			errorContains: "challenge file path cannot be empty",
		},
		{
			name:          "Invalid extension",
			challengePath: "test.txt",
			setupFunc:     func(t *testing.T) (string, func()) { return "", func() {} },
			shouldError:   true,
			errorContains: "challenge file must have .key extension",
		},
		{
			name:          "Directory traversal attempt",
			challengePath: "../../../etc/passwd.key",
			setupFunc:     func(t *testing.T) (string, func()) { return "", func() {} },
			shouldError:   true,
			errorContains: "does not match expected directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			challengePath := tt.challengePath
			cleanup := func() {}

			if tt.setupFunc != nil {
				var tempPath string
				tempPath, cleanup = tt.setupFunc(t)
				if tempPath != "" {
					challengePath = tempPath
				}
			}
			defer cleanup()

			err := validateArcChallengePath(logger, challengePath)

			if tt.shouldError {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if tt.errorContains != "" && !containsIgnoreCase(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateArcChallengePath_WindowsEnvironmentExpansion(t *testing.T) {
	// This test specifically validates that Windows environment variables are expanded
	logger := logging.New(nil)

	// Test with a mock path that should fail validation due to directory mismatch
	// but should NOT fail due to unexpanded environment variables
	challengePath := `C:\SomeOtherPath\AzureConnectedMachineAgent\Tokens\test.key`

	err := validateArcChallengePath(logger, challengePath)

	// Should get directory mismatch error, not environment variable expansion error
	if err == nil {
		t.Fatalf("Expected error due to directory mismatch")
	}

	// The error should mention directory mismatch, not unexpanded environment variables
	if containsIgnoreCase(err.Error(), "${PROGRAMDATA}") {
		t.Errorf("Error message contains unexpanded environment variable: %v", err)
	}
}

// Helper function for case-insensitive string contains check
func containsIgnoreCase(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
