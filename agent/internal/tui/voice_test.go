package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/tools"
	"github.com/keytron/allan/agent/internal/voice"
)

func newTestTUIModel(t *testing.T) *Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ALLAN_SECRETS", "file")
	cfg := config.Default()
	ag := &agent.Agent{
		Tools: tools.NewRegistry(),
	}
	m := New(cfg, ag, "/tmp/ws", "0.4.11-voice")
	m.width = 110
	m.height = 32
	m.recalcLayout()
	return m
}

func TestVoiceDockRendersBelowInputAcrossStates(t *testing.T) {
	m := newTestTUIModel(t)

	// 1. Idle state: dock appears below input box and above status bar.
	viewIdle := m.View()
	inputIdx := strings.Index(viewIdle, "› ")
	voiceIdx := strings.Index(viewIdle, "◎ VOICE")
	if inputIdx < 0 || voiceIdx < 0 || voiceIdx <= inputIdx {
		t.Fatalf("expected ◎ VOICE dock below input box (inputIdx=%d, voiceIdx=%d):\n%s", inputIdx, voiceIdx, viewIdle)
	}

	// 2. Feed synthetic demo speech PCM into the analyzer and verify spectrum + Braille waveform animate.
	wav := voice.SynthesizeDemoSpeechWAV(500 * time.Millisecond)
	pcm, err := voice.DecodeWAVTo16kMono(wav)
	if err != nil {
		t.Fatal(err)
	}
	for off := 0; off+voice.CaptureFrameBytes <= len(pcm); off += voice.CaptureFrameBytes {
		m.voice.Analyzer.PushPCM16LE(pcm[off : off+voice.CaptureFrameBytes])
	}

	m.voice.State = VoiceRecording
	m.voice.CaptureMode = VoiceModePTT
	dockRec := m.renderVoiceDock()
	if !strings.Contains(dockRec, "● REC") || !strings.Contains(dockRec, "PTT") {
		t.Fatalf("expected ● REC PTT badge in recording dock:\n%s", dockRec)
	}
	// Check that non-flat spectrum bars appear when speech energy is present.
	hasRaisedBar := false
	for _, r := range []rune{'▂', '▃', '▄', '▅', '▆', '▇', '█'} {
		if strings.ContainsRune(dockRec, r) {
			hasRaisedBar = true
			break
		}
	}
	if !hasRaisedBar {
		t.Fatalf("expected active spectrum bars in recording dock:\n%s", dockRec)
	}

	// 3. Transcribing state
	m.voice.State = VoiceTranscribing
	dockSTT := m.renderVoiceDock()
	if !strings.Contains(dockSTT, "STT") {
		t.Fatalf("expected STT indicator in transcribing dock:\n%s", dockSTT)
	}

	// 4. Speaking (TTS) state
	m.voice.State = VoiceSpeaking
	m.voice.ActiveEngine = "allan-formant-synth"
	dockTTS := m.renderVoiceDock()
	if !strings.Contains(dockTTS, "◉ TTS") || !strings.Contains(dockTTS, "allan-formant-synth") {
		t.Fatalf("expected ◉ TTS and engine name in speaking dock:\n%s", dockTTS)
	}
}

func TestSpacebarDoesNotHijackNonEmptyInput(t *testing.T) {
	m := newTestTUIModel(t)
	m.textarea.SetValue("git")
	m.textarea.CursorEnd()

	_, consumed := m.handleVoiceKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if consumed {
		t.Fatal("Spacebar must not trigger voice recording when textarea is non-empty")
	}
	if m.voice.State != VoiceIdle {
		t.Fatalf("expected VoiceIdle, got %v", m.voice.State)
	}
}

func TestHoldSpaceRepeatTransitionsToPTTAndReleases(t *testing.T) {
	m := newTestTUIModel(t)
	// Simulate active recording started in candidate mode by first Space press.
	m.voice.State = VoiceRecording
	m.voice.CaptureMode = VoiceModeCandidate
	m.voice.LastSpaceAt = time.Now()
	m.voice.SpaceSeq = 1

	// A repeated Space event arrives within key-repeat window -> locks into PTT mode.
	cmd, consumed := m.handleVoiceKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if !consumed || cmd == nil {
		t.Fatal("expected repeated Space during recording to be consumed and schedule release check")
	}
	if m.voice.CaptureMode != VoiceModePTT {
		t.Fatalf("expected VoiceModePTT after key repeat, got %v", m.voice.CaptureMode)
	}
	if m.voice.SpaceRepeats != 1 {
		t.Fatalf("expected SpaceRepeats=1, got %d", m.voice.SpaceRepeats)
	}

	// When the release watchdog fires with matching sequence, PTT stops recording.
	seq := m.voice.SpaceSeq
	_, handled := m.handleVoiceMsg(voiceSpaceCheckMsg{seq: seq, initial: false})
	if !handled {
		t.Fatal("expected voiceSpaceCheckMsg to be handled")
	}
	if m.voice.State != VoiceIdle {
		t.Fatalf("expected recording to stop on PTT release check, state=%v", m.voice.State)
	}
}

func TestVoiceSlashCommandsAndCompletions(t *testing.T) {
	m := newTestTUIModel(t)

	items := m.completionsFor("/voice ")
	if len(items) < 5 {
		t.Fatalf("expected /voice completions, got %v", items)
	}

	// /voice off hides the dock
	m.handleSlash("/voice off")
	if m.Cfg.TUI.VoiceEnabled {
		t.Fatal("expected VoiceEnabled=false after /voice off")
	}
	if dock := m.renderVoiceDock(); dock != "" {
		t.Fatalf("expected empty dock when disabled, got %q", dock)
	}

	// /voice on restores the dock
	m.handleSlash("/voice on")
	if !m.Cfg.TUI.VoiceEnabled {
		t.Fatal("expected VoiceEnabled=true after /voice on")
	}

	// /voice auto toggles auto-send
	m.handleSlash("/voice auto on")
	if !m.Cfg.TUI.VoiceAutoSend {
		t.Fatal("expected VoiceAutoSend=true after /voice auto on")
	}

	// Transcription result with VoiceAutoSend=false puts text in input box
	m.Cfg.TUI.VoiceAutoSend = false
	m.voice.State = VoiceTranscribing
	m.handleVoiceMsg(voiceTranscribeResultMsg{text: "проверь статус гита", engine: "faster-whisper-local"})
	if got := m.textarea.Value(); got != "проверь статус гита" {
		t.Fatalf("textarea after transcription = %q, want %q", got, "проверь статус гита")
	}
}
