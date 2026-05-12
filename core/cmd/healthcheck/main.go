package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	client := http.Client{
		Timeout: 2 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/readyz", nil)
	if err != nil {
		os.Exit(1)
	}

	resp, err := client.Do(req)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
