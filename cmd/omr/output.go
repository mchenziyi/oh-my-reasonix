package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func writeJSONValue(path string, label string, value interface{}) error {
	writer := os.Stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if path != "" {
		fmt.Printf("%s report: %s\n", label, path)
	}
	return nil
}
