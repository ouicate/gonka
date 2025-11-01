package main

import (
	"log"
	"os"

	notebooks "notebooks"
)

func main() {
	baseURL := os.Getenv("ARCHIVE_NODE_URL")
	if baseURL == "" {
		baseURL = notebooks.DefaultArchiveNodeURL
	}

	if err := notebooks.RunEpochScript(baseURL, os.Stdout); err != nil {
		log.Fatalf("epoch script failed: %v", err)
	}
}

