package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/arch-err/calemdar/internal/serve"
	"github.com/arch-err/calemdar/internal/store"
	"github.com/spf13/cobra"
)

func runServe(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	s, err := store.Open(v)
	if err != nil {
		return err
	}
	defer s.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return serve.Run(ctx, serve.Options{Vault: v, Store: s})
}
