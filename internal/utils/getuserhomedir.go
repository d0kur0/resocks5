package utils

import (
	"fmt"
	"os"
)

func GetUserHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home dir: %w", err)
	}
	return home, nil
}
