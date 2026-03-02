package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ValidateEraseUserDataDir(path string) error {
	cleaned := filepath.Clean(path)
	if strings.TrimSpace(cleaned) == "" {
		return fmt.Errorf("refusing to erase empty user data directory path")
	}
	if filepath.Dir(cleaned) == cleaned {
		return fmt.Errorf("refusing to erase filesystem root path: %s", cleaned)
	}

	cwd, err := os.Getwd()
	if err == nil && filepath.Clean(cwd) == cleaned {
		return fmt.Errorf("refusing to erase current working directory: %s", cleaned)
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && filepath.Clean(homeDir) == cleaned {
		return fmt.Errorf("refusing to erase home directory: %s", cleaned)
	}

	return nil
}
