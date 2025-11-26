package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Printf("Application built and run at: %s\n", time.Now().Format(time.RFC3339))
	fmt.Print("ciao")
	fmt.Print(" world")
}
