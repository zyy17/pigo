package coding

import (
	"encoding/json"
	"fmt"
	"strings"
)

func decodeArgs(raw []byte, dest any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fmt.Errorf("missing arguments")
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}
	return nil
}

func requirePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}
	return nil
}

func missingPath(path string) error {
	return fmt.Errorf("file not found: %s", path)
}
