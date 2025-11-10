package main

import (
	"context"

	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	httpcmd "github.com/xucx/gox/internal/cmd/http"
	"github.com/xucx/gox/internal/version"
)

var (
	cmd = &cobra.Command{
		Use:           "gox",
		Short:         "gox",
		Version:       version.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Usage()
		},
	}
)

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	cobra.CheckErr(cmd.ExecuteContext(ctx))
}

func init() {
	cmd.AddCommand(httpcmd.HttpCmd)
}
