package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://allan.keytron-prime.org":  "wss://allan.keytron-prime.org/ws/allan/worker/",
		"http://127.0.0.1:8001/":           "ws://127.0.0.1:8001/ws/allan/worker/",
		"https://keytron-prime.org/pair/x": "wss://keytron-prime.org/ws/allan/worker/",
	} {
		got, err := URL(in)
		if err != nil || got != want {
			t.Errorf("URL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := URL("ftp://x"); err == nil {
		t.Error("ftp scheme accepted")
	}
}

// fakeSite plays the KEYTRON Prime end: it accepts one worker, sends the given
// requests and collects every frame the worker sends back.
func fakeSite(t *testing.T, token string, reqs []frame) (*httptest.Server, chan frame) {
	got := make(chan frame, 256)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != Path {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		for _, f := range reqs {
			raw, _ := json.Marshal(f)
			if c.Write(ctx, websocket.MessageText, raw) != nil {
				return
			}
		}
		for {
			_, raw, err := c.Read(ctx)
			if err != nil {
				return
			}
			var f frame
			_ = json.Unmarshal(raw, &f)
			got <- f
		}
	}))
	return srv, got
}

// parked keeps frames of other requests: they are served concurrently, so
// their frames interleave with the ones being collected.
var parked = map[string][]frame{}

func collect(t *testing.T, got chan frame, id string) (status int, body string, endErr string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		var f frame
		if q := parked[id]; len(q) > 0 {
			f, parked[id] = q[0], q[1:]
		} else {
			select {
			case f = <-got:
			case <-deadline:
				t.Fatalf("no end frame for %s", id)
			}
		}
		if f.ID != id {
			parked[f.ID] = append(parked[f.ID], f)
			continue
		}
		switch f.T {
		case "head":
			status = f.Status
		case "data":
			body += string(f.B64)
		case "end":
			return status, body, f.Error
		}
	}
}

func TestRequestsGoThroughHandler(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "%s %s %s", r.Method, r.Header.Get("X-Allan-Client"), buf[:n])
	})
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		f := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "event: delta\ndata: %d\n\n", i)
			f.Flush()
		}
	})
	srv, got := fakeSite(t, "tok", []frame{
		{T: "req", ID: "a", Method: "POST", Path: "/echo", Headers: map[string]string{"X-Allan-Client": "u7"}, B64: []byte(`{"x":1}`)},
		{T: "req", ID: "b", Method: "GET", Path: "/sse"},
		{T: "req", ID: "c", Method: "GET", Path: "/missing"},
	})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	up := make(chan bool, 4)
	go Run(ctx, Options{Server: srv.URL, Token: "tok", Version: "test", Handler: mux,
		Status: func(ok bool, err error) { up <- ok }})
	if !<-up {
		t.Fatal("tunnel did not come up")
	}

	if st, body, _ := collect(t, got, "a"); st != http.StatusCreated || body != `POST u7 {"x":1}` {
		t.Errorf("echo: %d %q", st, body)
	}
	if st, body, _ := collect(t, got, "b"); st != 200 || strings.Count(body, "event: delta") != 3 {
		t.Errorf("sse: %d %q", st, body)
	}
	if st, _, _ := collect(t, got, "c"); st != http.StatusNotFound {
		t.Errorf("missing route: %d", st)
	}
}

func TestCancelStopsTheHandler(t *testing.T) {
	stopped := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(stopped)
	})
	srv, _ := fakeSite(t, "tok", []frame{{T: "req", ID: "s", Path: "/slow"}, {T: "cancel", ID: "s"}})
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Run(ctx, Options{Server: srv.URL, Token: "tok", Handler: mux})
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel frame did not cancel the request context")
	}
}

func TestWrongTokenIsReported(t *testing.T) {
	srv, _ := fakeSite(t, "right", nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	errs := make(chan error, 1)
	go Run(ctx, Options{Server: srv.URL, Token: "wrong", Handler: http.NewServeMux(),
		Status: func(ok bool, err error) {
			select {
			case errs <- err:
			default:
			}
		}})
	if err := <-errs; err != errRejected {
		t.Fatalf("want errRejected, got %v", err)
	}
}
