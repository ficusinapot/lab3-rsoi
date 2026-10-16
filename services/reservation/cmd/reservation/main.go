package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
	"lab2/platform/config"
	serviceconfig "lab2/reservation/internal/config"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := config.NewCommand("reservation", func(cmd *cobra.Command, cfg serviceconfig.Config) error {
		return Run(cmd.Context(), cfg)
	})
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
