package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"resocks5/internal/consts"
)

func GetAppConfigDir() (string, error) {
	homeDir, err := GetUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home dir: %w", err)
	}

	appConfigDir := filepath.Join(homeDir, consts.AppConfigDir)
	if _, err := os.Stat(appConfigDir); os.IsNotExist(err) {
		os.MkdirAll(appConfigDir, 0755)
	}

	return appConfigDir, nil
}
