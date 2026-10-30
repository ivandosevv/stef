package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type presentation struct {
	View string `json:"view"`
}

// The host choice belongs to stef, independent of the MCP client's configuration.
func projectPresentation(repo, requested string) (string, error) {
	if requested != "" {
		if requested != "native" && requested != "ide" {
			return "", fmt.Errorf("--view must be native or ide")
		}
		return requested, nil
	}
	root, err := os.OpenRoot(repo)
	if err != nil {
		return "", err
	}
	defer root.Close()
	file, err := root.Open(".stef/presentation.json")
	if os.IsNotExist(err) {
		return "native", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	var value presentation
	if err := json.NewDecoder(io.LimitReader(file, 1024)).Decode(&value); err != nil || (value.View != "native" && value.View != "ide") {
		return "", fmt.Errorf("invalid stef presentation; use --view native or --view ide to reset it")
	}
	return value.View, nil
}

func savePresentation(repo, view string) error {
	root, err := os.OpenRoot(repo)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(".stef", 0700); err != nil {
		return err
	}
	name := fmt.Sprintf(".stef/presentation-%d.tmp", time.Now().UnixNano())
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	err = json.NewEncoder(file).Encode(presentation{View: view})
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(name, ".stef/presentation.json")
}
