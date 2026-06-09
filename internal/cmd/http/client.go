package http

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	ClientOptions = RunClientOptions{}
	ClientCmd     = &cobra.Command{
		Use:   "client",
		Short: "client",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ClientOptions.Url = args[0]
			return RunClient(cmd.Context(), ClientOptions)
		},
	}
)

func init() {
	flags := ClientCmd.Flags()
	flags.StringVarP(&ClientOptions.Method, "method", "x", "GET", "http method")
	flags.BoolVarP(&ClientOptions.Keepalive, "keepalive", "k", false, "keep alive")
	flags.IntVarP(&ClientOptions.Timeout, "timeout", "t", 30, "time out")
	flags.StringArrayVarP(&ClientOptions.Headers, "header", "H", []string{}, "http header")
	flags.StringVarP(&ClientOptions.Body, "body", "d", "", "http body")
}

type RunClientOptions struct {
	Method    string
	Url       string
	Keepalive bool
	Timeout   int
	Headers   []string
	Body      string
}

func RunClient(ctx context.Context, opts RunClientOptions) error {

	c := &http.Client{
		Timeout: time.Duration(opts.Timeout) * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        0,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(opts.Method, opts.Url, strings.NewReader(opts.Body))
	if err != nil {
		return err
	}
	for _, header := range opts.Headers {
		items := strings.SplitN(header, ":", 2)
		if len(items) == 2 {
			req.Header.Add(items[0], items[1])
		} else {
			req.Header.Add(items[0], "")
		}
	}

	res, err := c.Do(req)
	if err != nil {
		fmt.Println("Error: ", err)
		return err
	}
	defer res.Body.Close()

	fmt.Printf("< status_code: %d\n", res.StatusCode)
	for k, v := range res.Header {
		fmt.Printf("< Header %s: %s\n", k, strings.Join(v, " "))
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("Error: read body fail: %s\n", err)
		return err
	}

	fmt.Printf("< body: %s\n", string(body))

	return nil
}
