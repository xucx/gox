package http

import (
	"github.com/spf13/cobra"
)

var (
	HttpCmd = &cobra.Command{
		Use:   "http",
		Short: "http",
	}
)

func init() {
	HttpCmd.AddCommand(HttpClientCmd)
}
