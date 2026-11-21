package backend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Уровень размышлений уходит в нужном поле, рассуждения возвращаются, а
// модель, которая параметр не знает, получает повтор без него.
func TestOpenAIEffortAndReasoning(t *testing.T) {
	var bodies []map[string]any
	rejectEffort := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &b)
		bodies = append(bodies, b)
		if rejectEffort && b["reasoning_effort"] != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"Unrecognized request argument supplied: reasoning_effort"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok","reasoning_content":"думаю…"}}]}`))
	}))
	defer srv.Close()
	ctx := context.Background()
	msgs := []Message{{Role: RoleUser, Content: "hi"}}

	o := NewOpenAILike("nvidia", srv.URL, "", "m")
	o.SetEffort("max")
	resp, err := o.Chat(ctx, msgs, nil)
	if err != nil || resp.Reasoning != "думаю…" || bodies[0]["reasoning_effort"] != "high" {
		t.Fatalf("resp=%+v err=%v body=%v", resp, err, bodies[0])
	}

	or := NewOpenAILike("openrouter", srv.URL, "", "m")
	or.SetEffort("off")
	if _, err := or.Chat(ctx, msgs, nil); err != nil {
		t.Fatal(err)
	}
	if r, _ := bodies[1]["reasoning"].(map[string]any); r["enabled"] != false || bodies[1]["reasoning_effort"] != nil {
		t.Fatalf("openrouter body: %v", bodies[1])
	}

	rejectEffort = true
	bodies = nil
	o.SetEffort("low")
	if _, err := o.Chat(ctx, msgs, nil); err != nil {
		t.Fatalf("must retry without effort: %v", err)
	}
	if len(bodies) != 2 || bodies[1]["reasoning_effort"] != nil {
		t.Fatalf("retry bodies: %v", bodies)
	}
	if _, err := o.Chat(ctx, msgs, nil); err != nil || len(bodies) != 3 {
		t.Fatalf("after rejection effort must not be sent again: %d %v", len(bodies), err)
	}

	bodies = nil
	plain := NewOpenAILike("nvidia", srv.URL, "", "m")
	if _, err := plain.Chat(ctx, msgs, nil); err != nil || bodies[0]["reasoning_effort"] != nil {
		t.Fatalf("no level — no parameter: %v", bodies[0])
	}
}
