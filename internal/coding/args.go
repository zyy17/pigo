package coding

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

func decodeArgs(raw []byte, dest any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return errors.New("missing arguments")
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}
	return nil
}

func requirePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is required")
	}
	return nil
}

// statFile stats abs. A missing path or a directory is an error named with path.
func statFile(path, abs string) (os.FileInfo, error) {
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, missingPath(path)
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}
	return info, nil
}

func missingPath(path string) error {
	return fmt.Errorf("file not found: %s", path)
}
