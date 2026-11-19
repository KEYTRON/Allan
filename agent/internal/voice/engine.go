package voice

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/serve"
)

// Transcribe converts recorded 16 kHz mono WAV audio into text, prioritizing
// remote user PCs ("Мои ПК") and Cloud BYOK models over local VRAM-heavy engines
// unless a local engine was explicitly selected.
func Transcribe(ctx context.Context, cfg *config.Config, wav []byte, lang string) (text string, engine string, err error) {
	spec := strings.TrimSpace(cfg.TUI.VoiceSTTModel)
	autoMode := spec == ""
	if autoMode {
		// облако и свои ПК по очереди; локальный движок — только если все упали
		txt, label, cErr, ok := autoTranscribe(ctx, cfg, wav, lang)
		if ok {
			return txt, label, nil
		}
		err = cErr
	}

	// Cloud BYOK / KEYTRON Prime / Remote User PC / Multimodal Audio path
	if spec != "" && !strings.HasPrefix(spec, "local:") && (strings.Contains(spec, ":") || strings.Contains(spec, "/")) {
		txt, label, cErr := executeCloudOrRemoteSTT(ctx, cfg, spec, lang, wav)
		if cErr == nil {
			return txt, label, nil
		}
		if !autoMode {
			return "", label, cErr
		}
		// In auto mode, if primary cloud/remote failed, fall through to local fallback
		err = cErr
	}

	localName := strings.TrimPrefix(spec, "local:")
	if localName == "faster-whisper-daemon" || (autoMode && localName == "") {
		if t, ok := tryWarmWhisperDaemon(ctx, wav, lang); ok {
			return t, "💻 faster-whisper-daemon", nil
		}
		if localName == "faster-whisper-daemon" {
			localName = ""
		}
	}
	txt, eng, lErr := serve.TranscribeAudio(ctx, cfg, localName, lang, wav)
	if lErr == nil {
		return txt, "💻 " + eng, nil
	}
	if err != nil {
		return "", engine, err
	}
	return "", eng, lErr
}

func tryWarmWhisperDaemon(ctx context.Context, wav []byte, lang string) (string, bool) {
	stateDir := os.Getenv("XDG_RUNTIME_DIR")
	if stateDir == "" {
		stateDir = os.TempDir()
	}
	sockPath := filepath.Join(stateDir, "claude-voice", "stt.sock")
	if _, err := os.Stat(sockPath); err != nil {
		return "", false
	}
	tmp, err := os.CreateTemp("", "allan-stt-sock-*.wav")
	if err != nil {
		return "", false
	}
	tmpPath := tmp.Name()
	_, _ = tmp.Write(wav)
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	var d net.Dialer
	d.Timeout = 2 * time.Second
	conn, err := d.DialContext(ctx, "unix", sockPath)
	if err != nil {
		return "", false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(45 * time.Second))

	l := lang
	if l == "" {
		l = "auto"
	}
	req, _ := json.Marshal(map[string]string{"wav": tmpPath, "lang": l})
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return "", false
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return "", false
	}
	var resp struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil || resp.Error != "" {
		return "", false
	}
	return strings.TrimSpace(resp.Text), true
}

// Synthesize generates speech audio (16 kHz mono WAV) from an assistant reply,
// prioritizing remote user PCs ("Мои ПК") and Cloud BYOK models over local engines.
func Synthesize(ctx context.Context, cfg *config.Config, text, lang string) (wav []byte, engine string, err error) {
	cleaned := CleanMarkdownForSpeech(text)
	if cleaned == "" {
		return nil, "", nil
	}
	spec := strings.TrimSpace(cfg.TUI.VoiceTTSModel)
	voiceID := strings.TrimSpace(cfg.TUI.VoiceTTSVoice)
	autoMode := spec == ""
	if autoMode {
		w, label, cErr, ok := autoSynthesize(ctx, cfg, cleaned, lang, voiceID)
		if ok {
			return w, label, nil
		}
		err = cErr
	}

	if spec != "" && !strings.HasPrefix(spec, "local:") && (strings.Contains(spec, ":") || strings.Contains(spec, "/")) {
		w, label, cErr := executeCloudOrRemoteTTS(ctx, cfg, spec, voiceID, lang, cleaned)
		if cErr == nil && len(w) > 0 {
			return w, label, nil
		}
		if !autoMode {
			return nil, label, cErr
		}
		err = cErr
	}

	localName := strings.TrimPrefix(spec, "local:")
	if localName != "espeak-ng" {
		w, eng, sErr := serve.SynthesizeAudio(ctx, cfg, localName, voiceID, lang, cleaned)
		if sErr == nil && len(w) > 0 {
			return w, "💻 " + eng, nil
		}
		if localName != "" {
			return nil, eng, sErr
		}
	}
	if bin, lookErr := exec.LookPath("espeak-ng"); lookErr == nil {
		v := voiceID
		if v == "" {
			v = lang
		}
		if w, eErr := espeakSynthesize(ctx, bin, cleaned, v); eErr == nil && len(w) > 0 {
			return w, "💻 espeak-ng", nil
		}
	}
	return wav, engine, err
}

func pickBestVoiceOption(voices []VoiceOption, lang, text string) string {
	if len(voices) == 0 {
		return ""
	}
	want := strings.ToLower(strings.TrimSpace(lang))
	if want == "" {
		want = "en"
		for _, r := range text {
			if unicode.IsLetter(r) {
				if r >= 0x0400 && r <= 0x04FF {
					want = "ru"
				}
				break
			}
		}
	}
	for _, v := range voices {
		lv := strings.ToLower(v.ID + " " + v.Name)
		if strings.Contains(lv, want) || (want == "ru" && (strings.Contains(lv, "марфа") || strings.Contains(lv, "may_"))) {
			return v.ID
		}
	}
	return voices[0].ID
}

func espeakSynthesize(ctx context.Context, bin, text, lang string) ([]byte, error) {
	v := strings.ToLower(strings.TrimSpace(lang))
	if v == "" {
		v = "ru"
		for _, r := range text {
			if unicode.IsLetter(r) {
				if r < 128 {
					v = "en"
				}
				break
			}
		}
	}
	tmp, err := os.CreateTemp("", "allan-espeak-*.wav")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	cmd := exec.CommandContext(ctx, bin, "-v", v, "-s", "165", "-w", tmpPath, text)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(tmpPath)
}

var (
	reCodeBlock  = regexp.MustCompile("(?s)```.*?```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reBullets    = regexp.MustCompile(`(?m)^\s*[-*+•]\s+`)
	reBoldItal   = regexp.MustCompile(`(\*\*|__|\*)`)
	reMultiSpace = regexp.MustCompile(`\s+`)
)

// CleanMarkdownForSpeech strips code blocks, markdown syntax, and long paths so
// the TTS voice reads concise natural prose.
func CleanMarkdownForSpeech(md string) string {
	s := reCodeBlock.ReplaceAllString(md, " ")
	s = reLink.ReplaceAllString(s, "$1")
	s = reInlineCode.ReplaceAllString(s, "$1")
	s = reHeading.ReplaceAllString(s, "")
	s = reBullets.ReplaceAllString(s, "")
	s = reBoldItal.ReplaceAllString(s, "")
	s = strings.TrimSpace(reMultiSpace.ReplaceAllString(s, " "))
	// Limit length to keep narration snappy in CLI (~600 runes max).
	runes := []rune(s)
	if len(runes) > 600 {
		cut := 600
		for i := 580; i < len(runes) && i < 640; i++ {
			if runes[i] == '.' || runes[i] == '!' || runes[i] == '?' || unicode.IsSpace(runes[i]) {
				cut = i + 1
				break
			}
		}
		s = strings.TrimSpace(string(runes[:cut]))
	}
	return s
}

// SynthesizeDemoSpeechWAV generates a rich, natural-sounding formant-modulated
// synthetic voice signal (16 kHz mono s16le WAV) for `/voice test` so the
// spectrum analyzer, VU meter, and waveform visualizer can be tested deterministically.
func SynthesizeDemoSpeechWAV(duration time.Duration) []byte {
	if duration <= 0 {
		duration = 2400 * time.Millisecond
	}
	totalSamples := int(float64(SampleRate) * duration.Seconds())
	pcm := make([]byte, totalSamples*2)

	// Syllable cadence + vowel formant transitions (F1, F2, F3)
	type formantSet struct{ f0, f1, f2, f3 float64 }
	syllables := []formantSet{
		{135, 720, 1240, 2550}, // "А"
		{142, 440, 2050, 2750}, // "л" / "е"
		{128, 580, 1450, 2400}, // "а"
		{150, 320, 2280, 2950}, // "н" / "и"
		{138, 520, 1100, 2480}, // "о"
		{122, 650, 1580, 2600}, // "а"
	}

	var p0, p1, p2, p3 float64
	for i := 0; i < totalSamples; i++ {
		t := float64(i) / float64(SampleRate)
		prog := float64(i) / float64(max(1, totalSamples))
		sylIdx := int(prog*float64(len(syllables))) % len(syllables)
		fm := syllables[sylIdx]

		// Syllable amplitude envelope (natural speech bursts with brief consonant dips)
		sylPhase := math.Mod(prog*float64(len(syllables)), 1.0)
		env := math.Sin(math.Pi * sylPhase)
		env = math.Pow(math.Max(0, env), 0.55)
		// Global fade-in / fade-out
		if t < 0.08 {
			env *= t / 0.08
		} else if rem := duration.Seconds() - t; rem < 0.12 {
			env *= math.Max(0, rem/0.12)
		}

		vibrato := 1.0 + 0.018*math.Sin(2*math.Pi*5.4*t)
		p0 += 2 * math.Pi * fm.f0 * vibrato / float64(SampleRate)
		p1 += 2 * math.Pi * fm.f1 / float64(SampleRate)
		p2 += 2 * math.Pi * fm.f2 / float64(SampleRate)
		p3 += 2 * math.Pi * fm.f3 / float64(SampleRate)

		// Glottal pulse + formants + subtle aspiration harmonic
		sample := 0.42*math.Sin(p0) +
			0.22*math.Sin(2*p0) +
			0.28*math.Sin(p1) +
			0.18*math.Sin(p2) +
			0.11*math.Sin(p3) +
			0.06*math.Sin(p3*1.45)*math.Sin(p0)

		val := clampFloat(sample*env*0.58, -0.95, 0.95)
		s16 := int16(val * 32767.0)
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(s16))
	}
	return EncodeWAV16kMono(pcm, SampleRate)
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
