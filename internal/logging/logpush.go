package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// LogPusher wraps an slog.Handler and additionally pushes log records
// to VictoriaLogs via HTTP POST. Logs still go to the inner handler
// (stderr) and are also buffered and batch-posted to the remote endpoint.
//
// If the endpoint is unreachable, log push silently fails — no crash.
type LogPusher struct {
	inner    slog.Handler
	endpoint string
	client   *http.Client
	ch       chan []byte
	wg       sync.WaitGroup
	attrs    []slog.Attr
	group    string
}

// NewLogPusher creates a handler that pushes to VictoriaLogs.
// endpoint is the full insert URL, e.g.:
// http://localhost:9428/insert/jsonline?_stream_fields=source,level&_msg_field=msg&_time_field=time
func NewLogPusher(inner slog.Handler, endpoint string) *LogPusher {
	lp := &LogPusher{
		inner:    inner,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 5 * time.Second},
		ch:       make(chan []byte, 1000),
	}
	lp.wg.Add(1)
	go lp.batchLoop()
	return lp
}

// NewLogPusherFromURL constructs the full VictoriaLogs insert URL from a base URL.
func NewLogPusherFromURL(inner slog.Handler, baseURL string) *LogPusher {
	endpoint := baseURL + "/insert/jsonline?_stream_fields=source,level&_msg_field=msg&_time_field=time"
	return NewLogPusher(inner, endpoint)
}

func (lp *LogPusher) Enabled(ctx context.Context, level slog.Level) bool {
	return lp.inner.Enabled(ctx, level)
}

func (lp *LogPusher) Handle(ctx context.Context, r slog.Record) error {
	// Always pass to inner handler (stderr output)
	err := lp.inner.Handle(ctx, r)

	// Build JSON log line for VictoriaLogs
	entry := map[string]any{
		"time":   r.Time.Format(time.RFC3339Nano),
		"level":  r.Level.String(),
		"msg":    r.Message,
		"source": "symphony",
	}

	// Add all attributes
	r.Attrs(func(a slog.Attr) bool {
		entry[a.Key] = a.Value.Any()
		return true
	})
	for _, a := range lp.attrs {
		entry[a.Key] = a.Value.Any()
	}
	if lp.group != "" {
		entry["group"] = lp.group
	}

	data, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		return err
	}

	// Non-blocking send to channel
	select {
	case lp.ch <- data:
	default:
		// Channel full, drop log line (don't block the caller)
	}

	return err
}

func (lp *LogPusher) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogPusher{
		inner:    lp.inner.WithAttrs(attrs),
		endpoint: lp.endpoint,
		client:   lp.client,
		ch:       lp.ch,
		attrs:    append(lp.attrs, attrs...),
		group:    lp.group,
	}
}

func (lp *LogPusher) WithGroup(name string) slog.Handler {
	return &LogPusher{
		inner:    lp.inner.WithGroup(name),
		endpoint: lp.endpoint,
		client:   lp.client,
		ch:       lp.ch,
		attrs:    lp.attrs,
		group:    name,
	}
}

// Close flushes remaining logs and stops the background goroutine.
func (lp *LogPusher) Close() {
	close(lp.ch)
	lp.wg.Wait()
}

// batchLoop reads from the channel and sends batches to VictoriaLogs.
func (lp *LogPusher) batchLoop() {
	defer lp.wg.Done()

	var batch [][]byte
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-lp.ch:
			if !ok {
				// Channel closed, flush remaining
				if len(batch) > 0 {
					lp.sendBatch(batch)
				}
				return
			}
			batch = append(batch, data)
			if len(batch) >= 50 {
				lp.sendBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				lp.sendBatch(batch)
				batch = nil
			}
		}
	}
}

func (lp *LogPusher) sendBatch(batch [][]byte) {
	var buf bytes.Buffer
	for _, line := range batch {
		buf.Write(line)
		buf.WriteByte('\n')
	}

	resp, err := lp.client.Post(lp.endpoint, "application/x-ndjson", &buf)
	if err != nil {
		// Silently fail — VictoriaLogs might not be running
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Log to stderr (not through slog to avoid recursion)
		fmt.Fprintf(bytes.NewBuffer(nil), "logpush: VictoriaLogs returned %d\n", resp.StatusCode)
	}
}
