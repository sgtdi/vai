package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request for: %s\n", r.URL.Path)
	fmt.Fprintf(w, "Hello from your live-reloaded web server! The time is: %s", time.Now().Format(time.RFC1123))
}

func main() {
	http.HandleFunc("/", handler)
	log.Println("Server starting on port: 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
