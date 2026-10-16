package logging

import (
	"strings"

	"github.com/samber/oops"
)

type Config struct {
	Level  string       `mapstructure:"level"`
	Stdout StdoutConfig `mapstructure:"stdout"`
	Files  []FileConfig `mapstructure:"files"`
}

type StdoutConfig struct {
	Level string `mapstructure:"level"`
}

type FileConfig struct {
	Path             string `mapstructure:"path"`
	Level            string `mapstructure:"level"`
	MaxFileSizeMB    int    `mapstructure:"max_file_size_mb"`
	MaxFilesCount    int    `mapstructure:"max_files_count"`
	MaxFileAgeInDays int    `mapstructure:"max_file_age_in_days"`
}

func (c Config) Validate() error {
	if !validLevel(c.Level) {
		return oops.Errorf("invalid global log level")
	}
	if c.Stdout.Level != "" && !validLevel(c.Stdout.Level) {
		return oops.Errorf("invalid stdout log level")
	}
	for _, file := range c.Files {
		invalidDestination := strings.TrimSpace(file.Path) == "" || !validLevel(file.Level)
		invalidRotation := file.MaxFileSizeMB <= 0 || file.MaxFilesCount < 0 || file.MaxFileAgeInDays < 0
		if invalidDestination || invalidRotation {
			return oops.Errorf("invalid rotating log file configuration")
		}
	}
	return nil
}

func validLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}
