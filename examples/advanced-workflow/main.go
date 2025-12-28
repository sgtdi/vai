// example/advanced-workflow/main.go
package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

type AppConfig struct {
	Env      string
	DBString string
}

var startTime time.Time

func main() {
	startTime = time.Now()

	config := AppConfig{
		Env:      getEnv("APP_ENV", "production"),
		DBString: getEnv("DB_CONNECTION_STRING", "none"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", HomeHandler)
	mux.HandleFunc("/config", ConfigHandler(config))

	log.Printf("Starting server in '%s' mode, connected to '%s'", config.Env, config.DBString)
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// getEnv retrieves an environment variable or returns a fallback value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
