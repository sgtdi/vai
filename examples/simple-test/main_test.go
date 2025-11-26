package main

import (
	"os"
	"testing"
)

func TestAdd(t *testing.T) {
	if Add(2, 2) != 4 {
		t.Error("2 + 2 should equal 4")
	}
}

func TestIntegrationMode(t *testing.T) {
	if os.Getenv("TEST_MODE") != "integration" {
		t.Skip("Skipping integration test: TEST_MODE is not set to 'integration'")
	}

	t.Log("Running integration test...")
	if os.Getenv("DB_HOST") != "localhost" {
		t.Errorf("Expected DB_HOST to be 'localhost', but got '%s'", os.Getenv("DB_HOST"))
	}
}
