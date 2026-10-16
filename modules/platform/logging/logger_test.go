package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

const (
	debugLevel = "debug"
	infoLevel  = "info"
)

func TestFileLoggingUsesExactLevelAndJSON(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "nested", "debug.jsonl")
	logger, closeLogs, err := New(Config{
		Level: debugLevel,
		Files: []FileConfig{{
			Path: path, Level: debugLevel, MaxFileSizeMB: 1, MaxFilesCount: 1, MaxFileAgeInDays: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeLogs)
	logger.DebugContext(t.Context(), "debug record", slog.String("component", "test"))
	logger.InfoContext(t.Context(), "info record")
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	data, err := root.ReadFile("nested/debug.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) != 1 {
		t.Fatalf("records=%d, want only DEBUG", len(lines))
	}
	record := map[string]any{}
	if err := json.Unmarshal(lines[0], &record); err != nil {
		t.Fatal(err)
	}
	if record["level"] != "DEBUG" || record["component"] != "test" {
		t.Fatalf("unexpected log record: %v", record)
	}
}

func TestGlobalMinimumFiltersDestinations(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "debug.jsonl")
	logger, closeLogs, err := New(Config{
		Level: infoLevel,
		Files: []FileConfig{{Path: path, Level: debugLevel, MaxFileSizeMB: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeLogs)
	if logger.Enabled(t.Context(), slog.LevelDebug) {
		t.Fatal("global minimum must filter DEBUG")
	}
	logger.DebugContext(t.Context(), "filtered")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("filtered logging must not create a file: %v", err)
	}
}

func TestInvalidConfig(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		config Config
	}{
		{name: "missing level"},
		{name: "invalid stdout level", config: Config{Level: infoLevel, Stdout: StdoutConfig{Level: "unknown"}}},
		{name: "zero file size", config: Config{
			Level: infoLevel,
			Files: []FileConfig{{Path: "file", Level: debugLevel, MaxFileSizeMB: 0}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.config.Validate(); err == nil {
				t.Fatalf("configuration accepted: %+v", tc.config)
			}
		})
	}
}
