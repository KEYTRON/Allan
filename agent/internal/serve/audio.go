package serve

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/keytron/allan/agent/config"
)

// Speech on this machine for the site's chat: GET /v1/audio lists what this
// PC can do, POST /v1/audio/transcriptions and /v1/audio/speech run it. The
// request shapes follow OpenRouter's JSON audio API (base64 input_audio for
// transcription, raw audio bytes back from speech), so the site talks to a PC
// the same way it talks to a cloud.
//
// Engines are found without any setup: the whisper CLI, Ollama ASR models
// (Qwen3-ASR) and MLX TTS models already downloaded for mlx-audio. More can
// be listed as [[audio]] commands in config.toml. Only one job runs at a
// time: a laptop that loads two speech models at once starts swapping.

type audioEngine struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Engine   string   `json:"engine"` // command | ollama-asr | apple-dictation | piper
	Voices   []string `json:"voices,omitempty"`
	command  []string
	model    string // ollama-asr: model name in the local Ollama; apple-dictation: the helper app
	stdin    bool   // the text goes to the command's stdin (piper)
	wav      bool   // the engine reads only wav/mp3/m4a: convert the browser's webm first
	voiceDir string // piper: folder with <voice>.onnx
}

// qwen3TTSVoices are the speakers of the Qwen3-TTS CustomVoice models.
var qwen3TTSVoices = []string{"Ryan", "Vivian", "Serena", "Dylan", "Eric", "Aiden", "Uncle_Fu", "Ono_Anna", "Sohee"}

const ollamaLocal = "http://localhost:11434"

var (
	audioMu     sync.Mutex
	audioCache  []audioEngine
	audioCached time.Time
	audioJob    = make(chan struct{}, 1)
)

func audioEngines(cfg *config.Config) []audioEngine {
	audioMu.Lock()
	defer audioMu.Unlock()
	if time.Since(audioCached) < time.Minute && audioCache != nil {
		return audioCache
	}
	seen := map[string]bool{}
	var out []audioEngine
	add := func(e audioEngine) {
		if e.Name != "" && !seen[e.Name] {
			seen[e.Name] = true
			out = append(out, e)
		}
	}
	for _, a := range cfg.Audio {
		if (a.Kind == "stt" || a.Kind == "tts") && len(a.Command) > 0 {
			add(audioEngine{Name: a.Name, Kind: a.Kind, Engine: "command", Voices: a.Voices, command: a.Command})
		}
	}
	home, _ := os.UserHomeDir()
	// faster-whisper через whisper-ctranslate2 (ставит allan audio setup): в разы быстрее openai-whisper
	if bin, err := exec.LookPath("whisper-ctranslate2"); err == nil {
		for _, size := range []string{"small", "medium", "large-v3-turbo"} {
			add(audioEngine{Name: "faster-whisper-" + size, Kind: "stt", Engine: "command", command: []string{
				bin, "{input}", "--model", size, "--language", "{lang}", "--output_dir", "{outdir}",
				"--output_format", "txt", "--verbose", "False"}})
		}
	}
	if bin, err := exec.LookPath("whisper"); err == nil {
		sizes := []string{"small"}
		// models already downloaded run without a first-time download
		if pts, _ := filepath.Glob(filepath.Join(home, ".cache", "whisper", "*.pt")); len(pts) > 0 {
			for _, p := range pts {
				sizes = append(sizes, strings.TrimSuffix(filepath.Base(p), ".pt"))
			}
		}
		for _, size := range sizes {
			add(audioEngine{Name: "whisper-" + size, Kind: "stt", Engine: "command", command: []string{
				bin, "{input}", "--model", size, "--language", "{lang}", "--task", "transcribe",
				"--output_format", "txt", "--output_dir", "{outdir}", "--verbose", "False"}})
		}
	}
	for _, m := range ollamaModels() {
		if strings.Contains(strings.ToLower(m), "asr") {
			add(audioEngine{Name: "ollama/" + m, Kind: "stt", Engine: "ollama-asr", model: m})
		}
	}
	if app := AppleDictationApp(); app != "" {
		add(audioEngine{Name: "apple-dictation", Kind: "stt", Engine: "apple-dictation", model: app, wav: true})
	}
	if bin := findBin("piper", filepath.Join(home, ".local", "bin")); bin != "" {
		dir := PiperVoiceDir()
		if onnx, _ := filepath.Glob(filepath.Join(dir, "*.onnx")); len(onnx) > 0 {
			var voices []string
			for _, f := range onnx {
				voices = append(voices, strings.TrimSuffix(filepath.Base(f), ".onnx"))
			}
			add(audioEngine{Name: "piper", Kind: "tts", Engine: "piper", Voices: voices, voiceDir: dir, stdin: true,
				command: []string{bin, "--model", "{voice_path}", "--output_file", "{outdir}/speech.wav"}})
		}
	}
	if voices := sayVoices(); len(voices) > 0 {
		add(audioEngine{Name: "macos-say", Kind: "tts", Engine: "command", Voices: voices,
			command: []string{"say", "-v", "{voice}", "-o", "{outdir}/speech.aiff", "-f", "{textfile}"}})
	}
	if bin := findBin("mlx_audio.tts.generate", filepath.Join(home, ".local", "bin")); bin != "" {
		repos, _ := filepath.Glob(filepath.Join(home, ".cache", "huggingface", "hub", "models--mlx-community--*TTS*"))
		for _, dir := range repos {
			repo := strings.Replace(strings.TrimPrefix(filepath.Base(dir), "models--"), "--", "/", 1)
			var voices []string
			if strings.Contains(repo, "Qwen3-TTS") && strings.Contains(repo, "CustomVoice") {
				voices = qwen3TTSVoices
			}
			add(audioEngine{Name: "mlx/" + strings.TrimPrefix(repo, "mlx-community/"), Kind: "tts", Engine: "command", Voices: voices,
				command: []string{bin, "--model", repo, "--text", "{text}", "--voice", "{voice}", "--lang_code", "{lang_name}",
					"--output_path", "{outdir}", "--file_prefix", "speech", "--join_audio", "--audio_format", "wav"}})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	audioCache, audioCached = out, time.Now()
	return out
}

// AppleDictationApp is the speech helper built by `allan audio setup` on macOS, or "".
func AppleDictationApp() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, _ := os.UserHomeDir()
	app := filepath.Join(home, ".allan", "helpers", "AllanSpeech.app")
	if _, err := os.Stat(filepath.Join(app, "Contents", "MacOS", "AllanSpeech")); err != nil {
		return ""
	}
	return app
}

// PiperVoiceDir is where `allan audio setup` puts Piper voices.
func PiperVoiceDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".allan", "voices", "piper")
}

var sayLine = regexp.MustCompile(`^(.*\S)\s+([a-z]{2,3})_[A-Z]{2,3}\s+#`)

// novelty voices of macOS say: fun, but useless for reading answers aloud
var sayNovelty = map[string]bool{"Albert": true, "Bad News": true, "Bahh": true, "Bells": true, "Boing": true, "Bubbles": true,
	"Cellos": true, "Wobble": true, "Zarvox": true, "Whisper": true, "Trinoids": true, "Organ": true, "Superstar": true,
	"Jester": true, "Good News": true, "Junior": true, "Kathy": true, "Fred": true, "Ralph": true}

// sayVoices lists the built-in macOS voices for Russian, English and German, Russian first.
func sayVoices() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	out, err := exec.Command("say", "-v", "?").Output()
	if err != nil {
		return nil
	}
	var ru, rest []string
	for _, line := range strings.Split(string(out), "\n") {
		m := sayLine.FindStringSubmatch(line)
		if m == nil || sayNovelty[strings.Fields(m[1])[0]] || sayNovelty[m[1]] {
			continue
		}
		switch m[2] {
		case "ru":
			ru = append(ru, m[1])
		case "en", "de":
			rest = append(rest, m[1])
		}
	}
	return append(ru, rest...)
}

func findBin(name string, extra ...string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range extra {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func ollamaModels() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ollamaLocal+"/api/tags", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&tags) != nil {
		return nil
	}
	out := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		out = append(out, m.Name)
	}
	return out
}

func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	engines := audioEngines(cfg)
	switch strings.TrimPrefix(r.URL.Path, "/v1/audio") {
	case "", "/":
		stt, tts := []audioEngine{}, []audioEngine{}
		for _, e := range engines {
			if e.Kind == "stt" {
				stt = append(stt, e)
			} else {
				tts = append(tts, e)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"stt": stt, "tts": tts})
	case "/transcriptions":
		s.transcribe(w, r, engines)
	case "/speech":
		s.speak(w, r, engines)
	default:
		writeErr(w, http.StatusNotFound, "нет такого пути")
	}
}

func pick(engines []audioEngine, kind, name string) *audioEngine {
	for _, e := range engines {
		if e.Kind == kind && e.Name == name {
			return &e
		}
	}
	return nil
}

// busy waits for the single audio slot; the request may give up first.
func busy(ctx context.Context) (func(), error) {
	select {
	case audioJob <- struct{}{}:
		return func() { <-audioJob }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var safeExt = regexp.MustCompile(`^[a-z0-9]{1,5}$`)

func (s *Server) transcribe(w http.ResponseWriter, r *http.Request, engines []audioEngine) {
	var req struct {
		Model      string `json:"model"`
		Language   string `json:"language"`
		InputAudio struct {
			Data   string `json:"data"`
			Format string `json:"format"`
		} `json:"input_audio"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 40<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "тело запроса должно быть JSON")
		return
	}
	e := pick(engines, "stt", req.Model)
	if e == nil {
		writeErr(w, http.StatusNotFound, "на этом компьютере нет модели распознавания "+req.Model)
		return
	}
	audio, err := base64.StdEncoding.DecodeString(req.InputAudio.Data)
	if err != nil || len(audio) == 0 {
		writeErr(w, http.StatusBadRequest, "input_audio.data — base64 аудио")
		return
	}
	ext := strings.ToLower(req.InputAudio.Format)
	if !safeExt.MatchString(ext) {
		ext = "webm"
	}
	dir, err := os.MkdirTemp("", "allan-stt-")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input."+ext)
	if err := os.WriteFile(input, audio, 0o600); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	release, err := busy(ctx)
	if err != nil {
		return
	}
	defer release()
	if e.wav && ext != "wav" && ext != "mp3" && ext != "m4a" {
		if input, err = ffmpeg(ctx, input, filepath.Join(dir, "input-16k.wav"), "-ac", "1", "-ar", "16000"); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	var text string
	if e.Engine == "ollama-asr" {
		text, err = ollamaASR(ctx, e.model, input, dir, req.Language)
	} else if e.Engine == "apple-dictation" {
		text, err = appleDictation(ctx, e.model, input, dir, req.Language)
	} else {
		text, err = runCommand(ctx, e.command, map[string]string{"input": input, "outdir": dir, "lang": req.Language,
			"lang_name": langName(req.Language, "")}, dir, ".txt")
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(text), "model": e.Name})
}

func (s *Server) speak(w http.ResponseWriter, r *http.Request, engines []audioEngine) {
	var req struct {
		Model          string `json:"model"`
		Input          string `json:"input"`
		Voice          string `json:"voice"`
		Language       string `json:"language"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || strings.TrimSpace(req.Input) == "" {
		writeErr(w, http.StatusBadRequest, "нужен JSON с текстом в input")
		return
	}
	e := pick(engines, "tts", req.Model)
	if e == nil {
		writeErr(w, http.StatusNotFound, "на этом компьютере нет модели синтеза "+req.Model)
		return
	}
	voice := req.Voice
	if voice == "" && len(e.Voices) > 0 {
		voice = e.Voices[0]
	}
	dir, err := os.MkdirTemp("", "allan-tts-")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	release, err := busy(ctx)
	if err != nil {
		return
	}
	defer release()
	textfile := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(textfile, []byte(req.Input), 0o600); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	vals := map[string]string{"text": req.Input, "textfile": textfile, "voice": voice, "outdir": dir,
		"lang": req.Language, "lang_name": langName(req.Language, req.Input)}
	if e.voiceDir != "" {
		vals["voice_path"] = filepath.Join(e.voiceDir, filepath.Base(voice)+".onnx")
	}
	stdin := ""
	if e.stdin {
		stdin = req.Input
	}
	if _, err := runCommandIn(ctx, e.command, vals, dir, "", stdin); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	file := newestAudio(dir)
	if file == "" {
		writeErr(w, http.StatusBadGateway, "движок синтеза не записал аудио")
		return
	}
	mime := "audio/wav"
	if req.ResponseFormat == "mp3" && !strings.HasSuffix(file, ".mp3") {
		if mp3, err := ffmpeg(ctx, file, filepath.Join(dir, "out.mp3"), "-codec:a", "libmp3lame", "-q:a", "4"); err == nil {
			file, mime = mp3, "audio/mpeg"
		}
	} else if strings.HasSuffix(file, ".mp3") {
		mime = "audio/mpeg"
	} else if !strings.HasSuffix(file, ".wav") {
		// say пишет AIFF: браузеры его не играют, отдаём WAV
		if wav, err := ffmpeg(ctx, file, filepath.Join(dir, "out.wav")); err == nil {
			file = wav
		}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", mime)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// runCommand fills the placeholders and runs the engine. The result is the
// first file with resultExt in dir when given, otherwise stdout.
func runCommand(ctx context.Context, tmpl []string, vals map[string]string, dir, resultExt string) (string, error) {
	return runCommandIn(ctx, tmpl, vals, dir, resultExt, "")
}

func runCommandIn(ctx context.Context, tmpl []string, vals map[string]string, dir, resultExt, stdin string) (string, error) {
	var args []string
	for i := 0; i < len(tmpl); i++ {
		a := tmpl[i]
		if (a == "{lang}" || a == "{voice}") && vals[strings.Trim(a, "{}")] == "" {
			if len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
				args = args[:len(args)-1] // drop "--language" together with the empty value
			}
			continue
		}
		for k, v := range vals {
			a = strings.ReplaceAll(a, "{"+k+"}", v)
		}
		args = append(args, a)
	}
	if len(args) == 0 {
		return "", fmt.Errorf("пустая команда")
	}
	if strings.HasPrefix(args[0], "~/") {
		home, _ := os.UserHomeDir()
		args[0] = filepath.Join(home, args[0][2:])
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(stderr.String())
		if len(tail) > 400 {
			tail = tail[len(tail)-400:]
		}
		return "", fmt.Errorf("%s: %v %s", filepath.Base(args[0]), err, tail)
	}
	if resultExt != "" {
		if files, _ := filepath.Glob(filepath.Join(dir, "*"+resultExt)); len(files) > 0 {
			b, err := os.ReadFile(files[0])
			return string(b), err
		}
	}
	return stdout.String(), nil
}

func newestAudio(dir string) string {
	var best string
	var when time.Time
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".wav", ".mp3", ".flac", ".ogg", ".m4a", ".aiff", ".aif":
			if info, err := d.Info(); err == nil && info.ModTime().After(when) && !strings.HasPrefix(filepath.Base(p), "input.") {
				best, when = p, info.ModTime()
			}
		}
		return nil
	})
	return best
}

func ffmpeg(ctx context.Context, in, out string, args ...string) (string, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("нужен ffmpeg")
	}
	full := append([]string{"-y", "-loglevel", "error", "-i", in}, args...)
	full = append(full, out)
	if b, err := exec.CommandContext(ctx, bin, full...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg: %v %s", err, strings.TrimSpace(string(b)))
	}
	return out, nil
}

var langNames = map[string]string{"ru": "russian", "en": "english", "de": "german", "fr": "french", "es": "spanish",
	"it": "italian", "pt": "portuguese", "zh": "chinese", "ja": "japanese", "ko": "korean"}

// langName turns an ISO code into the name speech tools expect; without a
// code it guesses Russian from Cyrillic in the text.
func langName(code, text string) string {
	if n, ok := langNames[strings.ToLower(code)]; ok {
		return n
	}
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			return "russian"
		}
	}
	if text != "" {
		return "english"
	}
	return ""
}

// ollamaASR runs a Qwen3-ASR style model in the local Ollama: 16 kHz mono WAV
// in 55-second pieces (the model takes up to a minute), each sent as an
// "image"; the answer looks like "language Russian<asr_text>text".
func ollamaASR(ctx context.Context, model, input, dir, lang string) (string, error) {
	if _, err := ffmpeg(ctx, input, filepath.Join(dir, "part%03d.wav"), "-ac", "1", "-ar", "16000",
		"-f", "segment", "-segment_time", "55"); err != nil {
		return "", err
	}
	parts, _ := filepath.Glob(filepath.Join(dir, "part*.wav"))
	sort.Strings(parts)
	var texts []string
	for _, part := range parts {
		wav, err := os.ReadFile(part)
		if err != nil {
			return "", err
		}
		messages := []map[string]any{{"role": "user", "content": "", "images": []string{base64.StdEncoding.EncodeToString(wav)}}}
		if n := langName(lang, ""); n != "" {
			// a prefilled answer pins the language; without it mixed speech may come back translated
			messages = append(messages, map[string]any{"role": "assistant", "content": "language " + strings.ToUpper(n[:1]) + n[1:] + "<asr_text>"})
		}
		body, _ := json.Marshal(map[string]any{"model": model, "messages": messages, "stream": false,
			"options": map[string]any{"num_ctx": 2048, "temperature": 0}})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, ollamaLocal+"/api/chat", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("ollama: %v", err)
		}
		var out struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Error string `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil || out.Error != "" {
			return "", fmt.Errorf("ollama: %v %s", err, out.Error)
		}
		text := out.Message.Content
		if i := strings.Index(text, "<asr_text>"); i >= 0 {
			text = text[i+len("<asr_text>"):]
		}
		texts = append(texts, strings.TrimSpace(text))
	}
	return strings.Join(texts, " "), nil
}

// appleDictation runs the on-device macOS speech recognizer through the
// helper app: macOS lets only an app with a usage description ask for speech
// recognition, so a bare binary would be refused. `open` makes the helper its
// own responsible process, so the permission granted once in `allan audio
// setup` applies; the result comes back in files.
func appleDictation(ctx context.Context, app, input, dir, lang string) (string, error) {
	out := filepath.Join(dir, "dictation.txt")
	args := []string{"-W", "-n", "-a", app, "--args", input, out}
	if lang != "" {
		args = append(args, lang)
	}
	if b, err := exec.CommandContext(ctx, "open", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("open: %v %s", err, strings.TrimSpace(string(b)))
	}
	if msg, err := os.ReadFile(out + ".err"); err == nil {
		return "", fmt.Errorf("диктовка macOS: %s", strings.TrimSpace(string(msg)))
	}
	text, err := os.ReadFile(out)
	if err != nil {
		return "", fmt.Errorf("диктовка macOS не вернула текст")
	}
	return string(text), nil
}

// AudioInfo is what `allan audio` prints about an engine.
type AudioInfo struct {
	Name, Kind string
	Voices     []string
}

// ListAudio returns the speech engines this machine offers to the site.
func ListAudio(cfg *config.Config) []AudioInfo {
	audioMu.Lock()
	audioCached = time.Time{} // fresh look: the user may have just installed something
	audioMu.Unlock()
	var out []AudioInfo
	for _, e := range audioEngines(cfg) {
		out = append(out, AudioInfo{Name: e.Name, Kind: e.Kind, Voices: e.Voices})
	}
	return out
}
