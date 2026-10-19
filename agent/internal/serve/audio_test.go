package serve

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAudioCommandsFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ALLAN_SECRETS", "file")
	t.Setenv("PATH", "/usr/bin:/bin") // no whisper/ffmpeg from the dev machine in the catalog
	dir := filepath.Join(home, ".allan")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`
[[audio]]
  name = "echo-stt"
  kind = "stt"
  command = ["sh", "-c", "test -s {input} && echo 'привет {lang}' > {outdir}/result.txt"]
[[audio]]
  name = "echo-tts"
  kind = "tts"
  voices = ["Ryan", "Vivian"]
  command = ["sh", "-c", "printf 'RIFF-{voice}-{lang_name}-{text}' > {outdir}/speech.wav"]
`), 0o600)
	audioCached = time.Time{}

	srv, _ := New(Options{Token: "tok"})
	h := srv.Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	var cat struct{ STT, TTS []audioEngine }
	json.Unmarshal(call("GET", "/v1/audio", "").Body.Bytes(), &cat)
	if len(cat.STT) != 1 || cat.STT[0].Name != "echo-stt" || len(cat.TTS) != 1 || len(cat.TTS[0].Voices) != 2 {
		t.Fatalf("catalog: %+v", cat)
	}

	body, _ := json.Marshal(map[string]any{"model": "echo-stt", "language": "ru",
		"input_audio": map[string]string{"data": base64.StdEncoding.EncodeToString([]byte("OggS...")), "format": "webm"}})
	rec := call("POST", "/v1/audio/transcriptions", string(body))
	var out struct{ Text string }
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out.Text != "привет ru" {
		t.Fatalf("stt: %d %s", rec.Code, rec.Body.String())
	}

	rec = call("POST", "/v1/audio/speech", `{"model":"echo-tts","input":"Привет"}`)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "audio/wav" || rec.Body.String() != "RIFF-Ryan-russian-Привет" {
		t.Fatalf("tts: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}

	if rec := call("POST", "/v1/audio/speech", `{"model":"nope","input":"x"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown model: %d", rec.Code)
	}
}

func TestEmptyLanguageDropsTheFlag(t *testing.T) {
	dir := t.TempDir()
	out, err := runCommand(context.Background(), []string{"sh", "-c", `echo "$@"`, "x", "--language", "{lang}", "--keep", "1"},
		map[string]string{"lang": ""}, dir, "")
	if err != nil || strings.TrimSpace(out) != "--keep 1" {
		t.Fatalf("got %q %v", out, err)
	}
}
