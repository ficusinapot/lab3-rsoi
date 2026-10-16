package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

const (
	rootAction    = "root"
	migrateAction = "migrate"
)

type testSettings struct {
	Timeout time.Duration `mapstructure:"timeout"`
}

func TestCommandYAML(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		subcommand string
		flagFirst  bool
		wantAction string
	}{
		{name: rootAction, flagFirst: true, wantAction: rootAction},
		{name: "flag before subcommand", subcommand: migrateAction, flagFirst: true, wantAction: migrateAction},
		{name: "flag after subcommand", subcommand: migrateAction, wantAction: migrateAction},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("timeout: 3s\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var action string
			var timeout time.Duration
			cmd := NewCommand("test", func(_ *cobra.Command, cfg testSettings) error {
				action, timeout = rootAction, cfg.Timeout
				return nil
			}, Action[testSettings]{Name: migrateAction, Run: func(_ *cobra.Command, cfg testSettings) error {
				action, timeout = migrateAction, cfg.Timeout
				return nil
			}})
			args := []string{"--config", path}
			if tc.subcommand != "" {
				if tc.flagFirst {
					args = append(args, tc.subcommand)
				} else {
					args = append([]string{tc.subcommand}, args...)
				}
			}
			cmd.SetArgs(args)
			if err := cmd.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			if action != tc.wantAction {
				t.Errorf("action = %q, want %q", action, tc.wantAction)
			}
			if timeout != 3*time.Second {
				t.Errorf("timeout = %s, want 3s", timeout)
			}
		})
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("unknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load[testSettings](path); err == nil {
		t.Fatal("unknown configuration key accepted")
	}
}

func TestCommandPreservesActionError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("timeout: 3s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("action failed")
	cmd := NewCommand("test", func(*cobra.Command, testSettings) error { return cause })
	cmd.SetArgs([]string{"--config", path})
	if err := cmd.ExecuteContext(t.Context()); !errors.Is(err, cause) {
		t.Fatalf("error = %v, want cause %v", err, cause)
	}
}
