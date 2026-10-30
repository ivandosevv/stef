package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func sessionExporter(root string) func(string) (string, error) {
	return func(data string) (string, error) {
		if len(data) > 64*1024*1024 || !json.Valid([]byte(data)) {
			return "", fmt.Errorf("invalid or oversized session")
		}
		confined, err := os.OpenRoot(root)
		if err != nil {
			return "", err
		}
		defer confined.Close()
		directory := filepath.Join(".stef", "exports")
		if err := confined.MkdirAll(directory, 0700); err != nil {
			return "", err
		}
		name := filepath.Join(directory, "stef-session-"+time.Now().Format("20060102-150405.000000000")+".json")
		file, err := confined.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return "", err
		}
		_, writeErr := file.WriteString(data)
		closeErr := file.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return filepath.Join(root, name), nil
	}
}
