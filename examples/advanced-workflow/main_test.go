// example/advanced-workflow/main_test.go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestHomeHandler checks the home handler for the correct status code and a non-empty body.
func TestHomeHandler(t *testing.T) {
	// Create a request to pass to our handler.
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(HomeHandler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	if rr.Body.String() == "" {
		t.Errorf("handler returned an empty body")
	}
}

// TestConfigHandler checks that the config handler returns the correct configuration in JSON format.
func TestConfigHandler(t *testing.T) {
	// Define a mock config for the test.
	mockConfig := AppConfig{
		Env:      "test",
		DBString: "mock-db-string",
	}

	req, err := http.NewRequest("GET", "/config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// The handler is a closure, so we call it to get the http.HandlerFunc.
	handler := ConfigHandler(mockConfig)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Decode the JSON response.
	var returnedConfig AppConfig
	if err := json.NewDecoder(rr.Body).Decode(&returnedConfig); err != nil {
		t.Fatalf("could not decode response JSON: %v", err)
	}

	// Compare the returned config with the mock config.
	if returnedConfig != mockConfig {
		t.Errorf("handler returned unexpected body: got %v want %v",
			returnedConfig, mockConfig)
	}
}

// TestGetEnv tests the helper function for reading environment variables.
func TestGetEnv(t *testing.T) {
	// Test case 1: Environment variable is set.
	os.Setenv("TEST_ENV_VAR", "test_value")
	result := getEnv("TEST_ENV_VAR", "fallback")
	if result != "test_value" {
		t.Errorf("getEnv failed: expected 'test_value', got '%s'", result)
	}
	os.Unsetenv("TEST_ENV_VAR")

	// Test case 2: Environment variable is not set, should return fallback.
	result = getEnv("TEST_ENV_VAR_UNSET", "fallback")
	if result != "fallback" {
		t.Errorf("getEnv failed: expected 'fallback', got '%s'", result)
	}
}
