package voice

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestAnalyzerDetectsFrequencyBandAndLevel(t *testing.T) {
	a := NewAnalyzer()
	// Generate a 1000 Hz sine wave (matches band 7: 1000 Hz) at 0.4 amplitude.
	samples := make([]float64, 512)
	for i := range samples {
		samples[i] = 0.4 * math.Sin(2*math.Pi*1000.0*float64(i)/float64(SampleRate))
	}
	var st FrameStats
	for i := 0; i < 4; i++ {
		st = a.PushSamples(samples)
	}
	if !st.Active {
		t.Fatalf("expected active signal for 0.4 amplitude sine wave, rms=%f", st.RMS)
	}
	if st.RMS < 0.2 {
		t.Fatalf("expected RMS >= 0.2, got %f", st.RMS)
	}
	if st.DBFS <= -50 || st.DBFS >= 0 {
		t.Fatalf("unexpected dBFS: %f", st.DBFS)
	}
	// Band 7 (1000 Hz) should have the highest energy among all bands.
	peakBand := 0
	for b := 1; b < NumBands; b++ {
		if st.Bands[b] > st.Bands[peakBand] {
			peakBand = b
		}
	}
	if peakBand != 7 {
		t.Fatalf("expected 1000 Hz tone to peak at band 7 (1000 Hz), got band %d (%.0f Hz): %v",
			peakBand, bandFreqs[peakBand], st.Bands)
	}

	// Test Decay brings energy down smoothly.
	prevRMS := st.RMS
	a.Decay()
	after := a.Snapshot()
	if after.RMS >= prevRMS {
		t.Fatalf("expected Decay to reduce RMS from %f, got %f", prevRMS, after.RMS)
	}
}

func TestWAVEncodeDecodeRoundTrip(t *testing.T) {
	wav := SynthesizeDemoSpeechWAV(600 * time.Millisecond)
	if len(wav) <= 44 {
		t.Fatalf("expected valid WAV buffer, got %d bytes", len(wav))
	}
	pcm, err := DecodeWAVTo16kMono(wav)
	if err != nil {
		t.Fatalf("DecodeWAVTo16kMono failed: %v", err)
	}
	wantBytes := int(0.6 * float64(SampleRate) * 2)
	if len(pcm) != wantBytes {
		t.Fatalf("decoded PCM bytes = %d, want %d", len(pcm), wantBytes)
	}

	// Feed demo speech into Analyzer and verify multiple formant bands light up.
	a := NewAnalyzer()
	for off := 0; off+CaptureFrameBytes <= len(pcm); off += CaptureFrameBytes {
		a.PushPCM16LE(pcm[off : off+CaptureFrameBytes])
	}
	st := a.Snapshot()
	lit := 0
	for _, b := range st.BandPeaks {
		if b > 0.15 {
			lit++
		}
	}
	if lit < 4 {
		t.Fatalf("expected synthetic demo voice to excite at least 4 formant bands, got %d (%v)", lit, st.BandPeaks)
	}
}

func TestCleanMarkdownForSpeech(t *testing.T) {
	in := "# Заголовок\nВот **важный** ответ с `inline_code` и [ссылкой](https://example.com).\n```go\nfmt.Println(\"skip me\")\n```\n- Первый пункт\n- Второй пункт"
	got := CleanMarkdownForSpeech(in)
	if strings.Contains(got, "fmt.Println") || strings.Contains(got, "```") || strings.Contains(got, "https://") {
		t.Fatalf("markdown not stripped properly: %q", got)
	}
	if !strings.Contains(got, "важный") || !strings.Contains(got, "inline_code") || !strings.Contains(got, "ссылкой") {
		t.Fatalf("prose content lost: %q", got)
	}
}

func TestSpeechLangFollowsLocaleNotUI(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ru_RU.UTF-8")
	if got := SpeechLang(""); got != "ru" {
		t.Fatalf("locale ru: got %q", got)
	}
	if got := SpeechLang("auto"); got != "" {
		t.Fatalf("auto: got %q", got)
	}
	if got := SpeechLang("DE"); got != "de" {
		t.Fatalf("explicit: got %q", got)
	}
	t.Setenv("LANG", "C.UTF-8")
	if got := SpeechLang(""); got != "" {
		t.Fatalf("C locale: got %q", got)
	}
}
