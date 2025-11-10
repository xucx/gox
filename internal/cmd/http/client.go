package http

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	httpClientOptions = RunClientOpts{}
	HttpClientCmd     = &cobra.Command{
		Use:   "client",
		Short: "client",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			httpClientOptions.Url = args[0]
			return RunClient(cmd.Context(), httpClientOptions)
		},
	}
)

func init() {
	flags := HttpClientCmd.Flags()
	flags.Uint32VarP(&httpClientOptions.Concurrency, "concurrency", "c", 1, "concurrency")
	flags.Uint32VarP(&httpClientOptions.Number, "number", "n", 1, "count")
	flags.StringVarP(&httpClientOptions.Method, "method", "x", "GET", "http method")
	flags.BoolVarP(&httpClientOptions.Keepalive, "keepalive", "k", false, "keep alive")
	flags.IntVarP(&httpClientOptions.Timeout, "timeout", "t", 30, "time out")
	flags.StringArrayVarP(&httpClientOptions.Headers, "header", "H", []string{}, "http header")
	flags.StringVarP(&httpClientOptions.Body, "body", "d", "", "http body")
}

const epsilon = 1e-9

type RunClientOpts struct {
	Concurrency uint32
	Number      uint32
	Method      string
	Url         string
	Keepalive   bool
	Timeout     int
	Headers     []string
	Body        string
}

func RunClient(ctx context.Context, options RunClientOpts) error {
	if options.Concurrency == 0 {
		options.Concurrency = 1
	}

	if options.Timeout < 1 {
		options.Timeout = 30
	}

	req, err := http.NewRequest(options.Method, options.Url, strings.NewReader(options.Body))
	if err != nil {
		return err
	}
	for _, header := range options.Headers {
		items := strings.SplitN(header, ":", 2)
		if len(items) == 2 {
			req.Header.Add(items[0], items[1])
		} else {
			req.Header.Add(items[0], "")
		}
	}

	ch := make(chan *sendResult, 10000)
	stopCh := make(chan bool)
	calculater := newClientCalculater(&options)
	waitCalculater := sync.WaitGroup{}
	waitCalculater.Add(1)
	go func() {
		defer waitCalculater.Done()
		calculater.run(ch, stopCh)
	}()

	waitWorker := sync.WaitGroup{}
	for i := 0; i < int(options.Concurrency); i++ {
		worker := newClientWorker(&options, ch, i, req, options.Number)

		waitWorker.Add(1)
		go func() {
			defer waitWorker.Done()
			worker.run(ctx)
		}()
	}
	waitWorker.Wait()
	stopCh <- true

	waitCalculater.Wait()

	return nil
}

type clientCalculater struct {
	options *RunClientOpts
	mu      sync.Mutex
	startAt time.Time
	last    *clientCaculaterData
	all     clientCaculaterData
	allTime []time.Duration
}

type clientCaculaterData struct {
	totalCount    int64
	succCount     int64
	failCount     int64
	totalTimeMs   float64
	succTimeMs    float64
	failTimeMs    float64
	minDurationMs float64
	maxDurationMs float64
	totalBytes    int64
	succBytes     int64
	failBytes     int64
}

func newClientCalculater(options *RunClientOpts) *clientCalculater {
	return &clientCalculater{
		options: options,
		startAt: time.Now(),
	}
}

func (c *clientCalculater) run(ch chan *sendResult, stopCh chan bool) {

	stopShow := make(chan bool)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopShow:
				c.show(true)
				return
			case <-ticker.C:
				c.show(false)
			}
		}
	}()

Loop:
	for {
		select {
		case <-stopCh:
			break Loop
		case result := <-ch:
			c.add(result)
		}
	}

	stopShow <- true

	wg.Wait()
}

func (c *clientCalculater) add(r *sendResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	duration := r.EndAt.Sub(r.StartAt)
	durationMs := float64(duration) / float64(time.Millisecond)
	c.allTime = append(c.allTime, duration)

	if c.last == nil {
		c.last = &clientCaculaterData{}
	}

	c.last.totalCount++
	if r.Err == nil && r.StatusCode == 200 {
		c.last.succCount++
		c.last.succTimeMs += durationMs
		if math.Abs(c.last.maxDurationMs) < epsilon || c.last.maxDurationMs < durationMs {
			c.last.maxDurationMs = durationMs
		}
		if math.Abs(c.last.minDurationMs) < epsilon || c.last.minDurationMs > durationMs {
			c.last.minDurationMs = durationMs
		}
		c.last.succBytes += int64(r.BodySize)
	} else {
		c.last.failCount++
		c.last.failTimeMs += durationMs
		c.last.failBytes += int64(r.BodySize)
	}
	c.last.totalTimeMs += durationMs
	c.last.totalBytes += int64(r.BodySize)

}

func (c *clientCalculater) show(final bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var (
		now = time.Now()
		qps int64
		avg float64
		p99 float64
		p95 float64
		p90 float64
	)

	if c.last != nil {
		if int64(c.last.totalTimeMs) > 0 {
			qps = int64(c.options.Concurrency) * c.last.succCount * 1000 / int64(c.last.totalTimeMs)
		}
		avg = float64(c.last.succTimeMs) / float64(c.last.succCount)

		fmt.Printf("Time(seconds): %-5d | QPS: %-8d | Count: %-8d %-8d %-8d | Duration(ms): %-10.3f %-10.3f %-10.3f | Recv(Bytes): %-10d\n",
			now.Sub(c.startAt)/time.Second,
			qps,
			c.last.totalCount,
			c.last.succCount,
			c.last.failCount,
			avg,
			c.last.maxDurationMs,
			c.last.minDurationMs,
			c.last.totalBytes,
		)

		// copy to all
		c.all.totalCount += c.last.totalCount
		c.all.succCount += c.last.succCount
		c.all.failCount += c.last.failCount
		c.all.totalTimeMs += c.last.totalTimeMs
		c.all.succTimeMs += c.last.succTimeMs
		c.all.failTimeMs += c.last.failTimeMs
		if math.Abs(c.all.minDurationMs) < epsilon || c.all.minDurationMs > c.last.minDurationMs {
			c.all.minDurationMs = c.last.minDurationMs
		}
		if math.Abs(c.all.maxDurationMs) < epsilon || c.all.maxDurationMs < c.last.maxDurationMs {
			c.all.maxDurationMs = c.last.maxDurationMs
		}
		c.all.totalBytes += c.last.totalBytes
		c.all.succBytes += c.last.succBytes
		c.all.failBytes += c.last.failBytes

		c.last = nil

	}

	if final {
		if int64(c.all.totalTimeMs) > 0 {
			qps = int64(c.options.Concurrency) * c.all.succCount * 1000 / int64(c.all.totalTimeMs)
		}
		avg = float64(c.all.succTimeMs) / float64(time.Millisecond)

		if len(c.allTime) > 0 {
			sort.Slice(c.allTime, func(i, j int) bool { return int64(c.allTime[i]) > int64(c.allTime[j]) })
			p99 = float64(c.allTime[int(float64(len(c.allTime))*0.99)]) / float64(time.Millisecond)
			p95 = float64(c.allTime[int(float64(len(c.allTime))*0.95)]) / float64(time.Millisecond)
			p90 = float64(c.allTime[int(float64(len(c.allTime))*0.90)]) / float64(time.Millisecond)
		}

		fmt.Printf("\n\n---------------------------\n")
		fmt.Printf("Concurrency   %d\n", c.options.Concurrency)
		fmt.Printf("Time(Second)  %.3f\n", now.Sub(c.startAt).Seconds())
		fmt.Printf("Send          %d (Success:%d, Fail:%d)\n", c.all.totalCount, c.all.succCount, c.all.failCount)
		fmt.Printf("Duration(ms)  %.3f (Avg: %.3f Max: %.3f, Min: %.3f, P99: %.3f P95: %.3f P90: %.3f)\n", avg, avg, c.all.maxDurationMs, c.all.minDurationMs, p99, p95, p90)
		fmt.Printf("Recv(Byte)    %d\n", c.all.totalBytes)
		fmt.Printf("QPS           %d\n", qps)
		fmt.Printf("---------------------------\n\n")
	}

}

type clientWorker struct {
	workerIndex int
	options     *RunClientOpts
	ch          chan *sendResult
	count       uint32
	client      *http.Client
	req         *http.Request
}

func newClientWorker(options *RunClientOpts, ch chan *sendResult, index int, req *http.Request, count uint32) *clientWorker {
	return &clientWorker{
		workerIndex: index,
		options:     options,
		ch:          ch,
		client:      newHttpClient(options),
		req:         req,
		count:       count,
	}
}

func (c *clientWorker) run(ctx context.Context) {
	for i := 0; c.count == 0 || i < int(c.count); i++ {
		if ctx.Err() != nil {
			break
		}

		c.ch <- c.send()

	}
}

type sendResult struct {
	WorkerIndex int
	Err         error
	StartAt     time.Time
	EndAt       time.Time
	StatusCode  int
	BodySize    int
}

func (c *clientWorker) send() (result *sendResult) {
	result = &sendResult{
		StartAt: time.Now(),
	}

	defer func() {
		result.EndAt = time.Now()
	}()

	client := c.getClient()
	res, err := client.Do(c.req)
	if err != nil {
		fmt.Println(err)
		result.Err = err
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		result.Err = err
		return
	}

	result.StatusCode = res.StatusCode
	result.BodySize = len(body)

	return
}

func (c *clientWorker) getClient() *http.Client {
	if c.options.Keepalive {
		return c.client
	}

	return newHttpClient(c.options)
}

func newHttpClient(options *RunClientOpts) *http.Client {
	return &http.Client{
		Timeout: time.Duration(options.Timeout) * time.Second,
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
}
