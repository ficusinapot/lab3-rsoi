package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"uuid"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	redisrepo "lab2/gateway/internal/redis/repos"
	"lab2/platform/config"
)

const (
	retryOperationName   = "retry-operation"
	inspectOperationName = "inspect-operation"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := config.NewCommand("gateway", func(cmd *cobra.Command, cfg Config) error {
		return Run(cmd.Context(), cfg)
	}, config.Action[Config]{Name: retryOperationName, Run: operationCommand},
		config.Action[Config]{Name: inspectOperationName, Run: operationCommand})
	for _, child := range command.Commands() {
		if child.Name() == retryOperationName || child.Name() == inspectOperationName {
			child.Flags().String("key", "", "operation UUID")
			if err := child.MarkFlagRequired("key"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
	}
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func operationCommand(cmd *cobra.Command, cfg Config) (err error) {
	rawKey, err := cmd.Flags().GetString("key")
	if err != nil {
		return oops.Wrap(err)
	}
	key, err := uuid.Parse(rawKey)
	if err != nil {
		return oops.Wrapf(err, "invalid operation UUID")
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.OpStream.StorageTimeout)
	defer cancel()
	store, err := redisrepo.New(ctx, cfg.Redis, cfg.OpStream)
	if err != nil {
		return oops.Wrap(err)
	}
	defer func() {
		err = oops.Wrap(errors.Join(err, store.Close()))
	}()
	if cmd.Name() == retryOperationName {
		return oops.Wrap(store.RetryFailed(ctx, key))
	}
	info, err := store.Inspect(ctx, key)
	if err != nil {
		return oops.Wrap(err)
	}
	output := map[string]any{
		"key": info.Key, "state": info.State, "owner": info.Owner,
		"attempt": info.Attempt, "until": info.Until, "error": info.Error,
	}
	if task := info.Task; task != nil {
		steps := make([]map[string]any, len(task.Steps))
		for i, action := range task.Steps {
			steps[i] = map[string]any{
				"type": fmt.Sprintf("%T", action.Data()), "payload": action.Data(),
			}
		}
		output["task"] = map[string]any{"kind": task.Kind, "steps": steps}
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return oops.Wrap(encoder.Encode(output))
}
