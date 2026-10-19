// Package tunnel keeps an outbound WebSocket from `allan serve` to KEYTRON
// Prime, so the site can reach this machine behind any NAT without a public
// address or Tailscale.
//
// The site sends HTTP requests over the socket and the worker answers them
// with its own HTTP handler, so the routes, token check and streaming are the
// same as on the local listener. Frames are JSON text:
//
//	site → worker:  {"t":"req","id","method","path","headers","b64"}  {"t":"cancel","id"}
//	worker → site:  {"t":"hello","version","machine_id"}  {"t":"head","id","status","headers"}
//	                {"t":"data","id","b64"}  {"t":"end","id","error"?}  {"t":"ping"}
//
// Bodies travel base64-encoded ([]byte in JSON). The worker never trusts a
// frame for authentication: every request still carries the worker token in
// its headers and goes through the same check as a direct HTTP call.
package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Path is where the site accepts workers.
const Path = "/ws/allan/worker/"

const (
	pingEvery  = 30 * time.Second
	flushAfter = 32 << 10 // send a data frame once this much is buffered
	readLimit  = 8 << 20
	maxBackoff = time.Minute
)

// Options configures the tunnel.
type Options struct {
	Server    string // https://allan.keytron-prime.org
	Token     string
	Version   string
	MachineID string
	Handler   http.Handler
	// Status reports connection changes for the console; may be nil.
	Status func(connected bool, err error)
}

// URL turns the site address into the tunnel endpoint.
func URL(server string) (string, error) {
	u, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("tunnel: плохой адрес сервера %q", server)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("tunnel: неизвестная схема %q", u.Scheme)
	}
	u.Path = Path
	return u.String(), nil
}

type frame struct {
	T         string            `json:"t"`
	ID        string            `json:"id,omitempty"`
	Method    string            `json:"method,omitempty"`
	Path      string            `json:"path,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	B64       []byte            `json:"b64,omitempty"`
	Status    int               `json:"status,omitempty"`
	Error     string            `json:"error,omitempty"`
	Version   string            `json:"version,omitempty"`
	MachineID string            `json:"machine_id,omitempty"`
}

// Run keeps the tunnel up until ctx is done, reconnecting with backoff.
func Run(ctx context.Context, opts Options) error {
	endpoint, err := URL(opts.Server)
	if err != nil {
		return err
	}
	if opts.Handler == nil || strings.TrimSpace(opts.Token) == "" {
		return errors.New("tunnel: нужны обработчик и токен")
	}
	backoff := time.Second
	for {
		started := time.Now()
		err := session(ctx, endpoint, opts)
		if ctx.Err() != nil {
			return nil
		}
		if opts.Status != nil {
			opts.Status(false, err)
		}
		if errors.Is(err, errRejected) {
			// a revoked or unknown token will not start working by retrying every second
			backoff = maxBackoff
		} else if time.Since(started) > 2*time.Minute {
			backoff = time.Second // the link was healthy for a while: reconnect quickly
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

var errRejected = errors.New("сайт не принял токен воркера: выполните allan connect заново")

func session(ctx context.Context, endpoint string, opts Options) error {
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(dialCtx, endpoint, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": {"Bearer " + opts.Token},
			"User-Agent":    {"allan/" + opts.Version},
		},
	})
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized) {
			return errRejected
		}
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(readLimit)

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	t := &tunnel{conn: conn, handler: opts.Handler, running: map[string]context.CancelFunc{}}
	if err := t.write(ctx, frame{T: "hello", Version: opts.Version, MachineID: opts.MachineID}); err != nil {
		return err
	}
	if opts.Status != nil {
		opts.Status(true, nil)
	}
	go func() {
		tick := time.NewTicker(pingEvery)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if t.write(ctx, frame{T: "ping"}) != nil {
					stop()
					return
				}
			}
		}
	}()
	defer t.cancelAll()
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == 4401 {
				return errRejected
			}
			return err
		}
		var f frame
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		switch f.T {
		case "req":
			t.start(ctx, f)
		case "cancel":
			t.cancel(f.ID)
		}
	}
}

type tunnel struct {
	conn    *websocket.Conn
	handler http.Handler

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

func (t *tunnel) write(ctx context.Context, f frame) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return t.conn.Write(wctx, websocket.MessageText, raw)
}

func (t *tunnel) start(parent context.Context, f frame) {
	if f.ID == "" || !strings.HasPrefix(f.Path, "/") {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	t.mu.Lock()
	t.running[f.ID] = cancel
	t.mu.Unlock()
	go func() {
		defer t.cancel(f.ID)
		t.serve(ctx, f)
	}()
}

func (t *tunnel) cancel(id string) {
	t.mu.Lock()
	cancel := t.running[id]
	delete(t.running, id)
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (t *tunnel) cancelAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, cancel := range t.running {
		cancel()
		delete(t.running, id)
	}
}

func (t *tunnel) serve(ctx context.Context, f frame) {
	method := f.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://tunnel"+f.Path, bytes.NewReader(f.B64))
	if err != nil {
		_ = t.write(ctx, frame{T: "end", ID: f.ID, Error: err.Error()})
		return
	}
	for k, v := range f.Headers {
		req.Header.Set(k, v)
	}
	req.RemoteAddr = "tunnel"
	w := &writer{t: t, ctx: ctx, id: f.ID, header: http.Header{}}
	func() {
		defer func() {
			if p := recover(); p != nil {
				w.err = fmt.Sprintf("воркер упал: %v", p)
			}
		}()
		t.handler.ServeHTTP(w, req)
	}()
	w.finish()
}

// writer is an http.ResponseWriter that streams the response as frames.
type writer struct {
	t      *tunnel
	ctx    context.Context
	id     string
	header http.Header
	status int
	buf    bytes.Buffer
	err    string
	broken bool
}

func (w *writer) Header() http.Header { return w.header }

func (w *writer) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	h := map[string]string{}
	for k := range w.header {
		h[k] = w.header.Get(k)
	}
	w.send(frame{T: "head", ID: w.id, Status: status, Headers: h})
}

func (w *writer) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.broken {
		return 0, errors.New("tunnel: соединение с сайтом оборвалось")
	}
	w.buf.Write(p)
	if w.buf.Len() >= flushAfter {
		w.Flush()
	}
	return len(p), nil
}

// Flush sends what is buffered: SSE handlers flush after every event.
func (w *writer) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.buf.Len() == 0 {
		return
	}
	data := bytes.Clone(w.buf.Bytes())
	w.buf.Reset()
	w.send(frame{T: "data", ID: w.id, B64: data})
}

func (w *writer) finish() {
	if w.status == 0 && w.err == "" {
		w.WriteHeader(http.StatusOK)
	}
	w.Flush()
	// the request may already be cancelled; the end frame must still go out
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = w.t.write(ctx, frame{T: "end", ID: w.id, Error: w.err})
}

func (w *writer) send(f frame) {
	if w.broken {
		return
	}
	if err := w.t.write(context.Background(), f); err != nil {
		w.broken = true
	}
}
