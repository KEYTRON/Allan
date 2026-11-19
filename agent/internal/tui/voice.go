package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/voice"
)

// VoiceState represents the active stage of the voice communication module.
type VoiceState int

const (
	VoiceIdle VoiceState = iota
	VoiceRecording
	VoiceTranscribing
	VoiceSpeaking
)

// VoiceCaptureMode distinguishes Hold-Space (Push-to-Talk) from Tap-Space / Ctrl+G (VAD).
type VoiceCaptureMode int

const (
	VoiceModeCandidate VoiceCaptureMode = iota // Initial Space press before we know if held or tapped
	VoiceModePTT                               // Space is being held down (key repeat observed)
	VoiceModeVAD                               // Space was tapped once or Ctrl+G pressed (stops on silence or Space/Enter)
)

// VoiceHub holds the TUI's voice communication state, DSP analyzer, recorder,
// player, and discovered Cloud BYOK / Remote PC / Local audio catalog.
type VoiceHub struct {
	Analyzer     *voice.Analyzer
	Recorder     *voice.Recorder
	Player       *voice.Player
	Catalog      []voice.AudioModelChoice
	State        VoiceState
	CaptureMode  VoiceCaptureMode
	SpaceSeq     int
	LastSpaceAt  time.Time
	SpaceRepeats int
	// зажатие Пробела в непустом поле (дописать голосом): серия пробелов подряд
	TypedSpaceRun int
	TypedSpaceAt  time.Time
	ActiveEngine  string
	StatusNote    string
	StatusNoteAt  time.Time
	TickRunning   bool
	SpinnerFrame  int
	SpeakCancel   context.CancelFunc
}

type voiceVisTickMsg struct{}
type voiceSpaceCheckMsg struct {
	seq     int
	initial bool
}
type voiceVADAutoStopMsg struct{}
type voiceTranscribeResultMsg struct {
	text   string
	engine string
	err    error
}
type voiceSpeakDoneMsg struct {
	engine string
	err    error
}
type voiceCatalogMsg struct {
	items    []voice.AudioModelChoice
	openKind string // "", "stt", "tts", "realtime", "voice"
}

var (
	spectrumBars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	brailleWave  = []rune{'⣀', '⣄', '⣤', '⣦', '⣶', '⣷', '⣿'}
	transSpin    = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	audioSectionTitles = []string{
		voice.SecMine:   "Мои ПК",
		voice.SecBYOK:   "BYOK",
		voice.SecDirect: "Напрямую",
		voice.SecLocal:  "Этот ПК",
	}
)

func newVoiceHub() *VoiceHub {
	a := voice.NewAnalyzer()
	return &VoiceHub{
		Analyzer: a,
		Recorder: voice.NewRecorder(a),
		Player:   voice.NewPlayer(a),
		State:    VoiceIdle,
	}
}

func (m *Model) refreshVoiceCatalog(force bool, openKind string) tea.Cmd {
	cfg := m.Cfg
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		items := voice.DiscoverCatalog(ctx, cfg, force)
		return voiceCatalogMsg{items: items, openKind: openKind}
	}
}

func voiceVisTickCmd() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(time.Time) tea.Msg {
		return voiceVisTickMsg{}
	})
}

func (m *Model) ensureVoiceTick() tea.Cmd {
	if m.voice == nil || m.voice.TickRunning {
		return nil
	}
	m.voice.TickRunning = true
	return voiceVisTickCmd()
}

func (m *Model) setVoiceNote(note string) {
	if m.voice == nil {
		return
	}
	m.voice.StatusNote = note
	m.voice.StatusNoteAt = time.Now()
}

// isSpaceKey reports whether the key event is a plain Spacebar press.
func isSpaceKey(msg tea.KeyMsg) bool {
	if msg.Paste {
		return false
	}
	if msg.Type == tea.KeySpace {
		return true
	}
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == ' '
}

// handleVoiceKey intercepts Space (on empty input for Hold-to-Talk / Tap-VAD),
// Ctrl+G (voice input anytime), and Esc/Space/Enter during active recording or TTS playback.
func (m *Model) handleVoiceKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.voice == nil {
		return nil, false
	}

	// Ctrl+G toggles voice recording regardless of whether the input box already has text.
	if msg.Type == tea.KeyCtrlG {
		if m.voice.State == VoiceRecording {
			return m.stopVoiceRecording(false), true
		}
		if m.voice.State == VoiceSpeaking {
			m.stopVoiceSpeaking()
		}
		if !m.thinking && m.voice.State == VoiceIdle {
			return m.startVoiceRecording(VoiceModeVAD), true
		}
		return nil, true
	}

	// While recording: handle Space (PTT hold repeat or VAD stop), Enter (stop & transcribe), Esc (cancel).
	if m.voice.State == VoiceRecording {
		if isSpaceKey(msg) {
			elapsed := time.Since(m.voice.LastSpaceAt)
			if (m.voice.CaptureMode == VoiceModeCandidate || m.voice.CaptureMode == VoiceModePTT) && elapsed < 780*time.Millisecond {
				// Key repeat arrived while holding Space -> lock into Push-to-Talk mode!
				m.voice.CaptureMode = VoiceModePTT
				m.voice.Recorder.SetVAD(false)
				m.voice.SpaceRepeats++
				m.voice.LastSpaceAt = time.Now()
				m.voice.SpaceSeq++
				seq := m.voice.SpaceSeq
				return tea.Tick(260*time.Millisecond, func(time.Time) tea.Msg {
					return voiceSpaceCheckMsg{seq: seq, initial: false}
				}), true
			}
			// In VAD mode, pressing Space a second time stops recording and transcribes.
			return m.stopVoiceRecording(false), true
		}
		switch msg.Type {
		case tea.KeyEnter:
			return m.stopVoiceRecording(false), true
		case tea.KeyEsc:
			return m.stopVoiceRecording(true), true
		}
		return nil, true
	}

	// If TTS is currently speaking and user presses Esc (when agent isn't running), stop TTS playback.
	if m.voice.State == VoiceSpeaking && msg.Type == tea.KeyEsc && !m.thinking && m.picker == nil {
		m.stopVoiceSpeaking()
		m.setVoiceNote(i18n.S("озвучка остановлена"))
		return nil, true
	}

	// Пробел в непустом поле: обычный символ, но зажатие узнаём по автоповтору —
	// второй пробел через 300–780 мс (задержка повтора), третий быстрее 150 мс
	// (частота повтора). Тогда убираем вставленные пробелы и пишем голос,
	// распознанное допишется к тексту. Ручной двойной пробел так не срабатывает.
	if m.Cfg.TUI.VoiceEnabled && voice.CanRecord() && !m.thinking && m.picker == nil && !m.popupOpen &&
		m.focus == FocusTUI && m.textarea.Value() != "" && m.voice.State != VoiceTranscribing {
		if !isSpaceKey(msg) {
			m.voice.TypedSpaceRun = 0
			return nil, false
		}
		gap := time.Since(m.voice.TypedSpaceAt)
		m.voice.TypedSpaceAt = time.Now()
		val := m.textarea.Value()
		switch {
		case m.voice.TypedSpaceRun == 1 && gap >= 300*time.Millisecond && gap < 780*time.Millisecond && strings.HasSuffix(val, " "):
			m.voice.TypedSpaceRun = 2
		case m.voice.TypedSpaceRun == 2 && gap < 150*time.Millisecond && strings.HasSuffix(val, "  "):
			m.voice.TypedSpaceRun = 0
			m.textarea.SetValue(strings.TrimSuffix(val, "  "))
			m.textarea.CursorEnd()
			if m.voice.State == VoiceSpeaking {
				m.stopVoiceSpeaking()
			}
			start := m.startVoiceRecording(VoiceModePTT)
			if m.voice.State != VoiceRecording {
				return start, true
			}
			seq := m.voice.SpaceSeq
			return tea.Batch(start, tea.Tick(260*time.Millisecond, func(time.Time) tea.Msg {
				return voiceSpaceCheckMsg{seq: seq, initial: false}
			})), true
		default:
			m.voice.TypedSpaceRun = 1
		}
		return nil, false
	}

	// Space on empty input when VoiceEnabled is on starts Push-to-Talk / Tap-VAD recording.
	if m.Cfg.TUI.VoiceEnabled && voice.CanRecord() && !m.thinking && m.picker == nil && !m.popupOpen &&
		m.focus == FocusTUI && m.textarea.Value() == "" && isSpaceKey(msg) {
		if m.voice.State == VoiceSpeaking {
			m.stopVoiceSpeaking()
		}
		if m.voice.State == VoiceIdle {
			return m.startVoiceRecording(VoiceModeCandidate), true
		}
	}

	return nil, false
}

func (m *Model) startVoiceRecording(mode VoiceCaptureMode) tea.Cmd {
	if m.voice == nil {
		return nil
	}
	if !m.Cfg.TUI.VoiceEnabled {
		m.Cfg.TUI.VoiceEnabled = true
	}
	m.voice.Analyzer.Reset()
	vadOn := mode == VoiceModeVAD
	err := m.voice.Recorder.Start(vadOn, func() {
		if globalProgram != nil {
			globalProgram.Send(voiceVADAutoStopMsg{})
		}
	})
	if err != nil {
		m.setVoiceNote(err.Error())
		m.appendMsg(Message{Kind: MsgWarn, Text: i18n.S("Микрофон недоступен: ") + err.Error()})
		return nil
	}
	m.voice.State = VoiceRecording
	m.voice.CaptureMode = mode
	m.voice.SpaceRepeats = 0
	m.voice.LastSpaceAt = time.Now()
	m.voice.SpaceSeq++
	seq := m.voice.SpaceSeq

	cmds := []tea.Cmd{m.ensureVoiceTick()}
	if mode == VoiceModeCandidate {
		cmds = append(cmds, tea.Tick(720*time.Millisecond, func(time.Time) tea.Msg {
			return voiceSpaceCheckMsg{seq: seq, initial: true}
		}))
	}
	return tea.Batch(cmds...)
}

func (m *Model) stopVoiceRecording(cancelOnly bool) tea.Cmd {
	if m.voice == nil || m.voice.State != VoiceRecording {
		return nil
	}
	wav, hasSpeech := m.voice.Recorder.Stop()
	m.voice.SpaceSeq++
	if cancelOnly {
		m.voice.State = VoiceIdle
		m.voice.Analyzer.Reset()
		m.setVoiceNote(i18n.S("запись отменена"))
		return nil
	}
	if len(wav) == 0 || !hasSpeech {
		m.voice.State = VoiceIdle
		m.voice.Analyzer.Reset()
		m.setVoiceNote(i18n.S("речь не услышана"))
		return nil
	}
	m.voice.State = VoiceTranscribing
	m.voice.SpinnerFrame = 0
	cfg := m.Cfg
	lang := voice.SpeechLang(cfg.TUI.VoiceLang)
	return tea.Batch(m.ensureVoiceTick(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		text, eng, err := voice.Transcribe(ctx, cfg, wav, lang)
		return voiceTranscribeResultMsg{text: text, engine: eng, err: err}
	})
}

func (m *Model) stopVoiceSpeaking() {
	if m.voice == nil {
		return
	}
	if m.voice.SpeakCancel != nil {
		m.voice.SpeakCancel()
		m.voice.SpeakCancel = nil
	}
	m.voice.Player.Stop()
	if m.voice.State == VoiceSpeaking {
		m.voice.State = VoiceIdle
	}
}

// startVoiceTTS synthesizes and plays the assistant's reply in the background
// while streaming its PCM frames into the visualizer below the input box.
func (m *Model) startVoiceTTS(replyText string) tea.Cmd {
	if m.voice == nil || !m.Cfg.TUI.VoiceEnabled || !m.Cfg.TUI.VoiceTTS {
		return nil
	}
	cleaned := voice.CleanMarkdownForSpeech(replyText)
	if cleaned == "" {
		return nil
	}
	m.stopVoiceSpeaking()
	ctx, cancel := context.WithCancel(context.Background())
	m.voice.SpeakCancel = cancel
	m.voice.State = VoiceSpeaking
	m.voice.ActiveEngine = m.preferredTTSEngineLabel()
	cfg := m.Cfg
	lang := voice.SpeechLang(cfg.TUI.VoiceLang)

	return tea.Batch(m.ensureVoiceTick(), func() tea.Msg {
		defer cancel()
		wav, eng, err := voice.Synthesize(ctx, cfg, cleaned, lang)
		if err != nil || len(wav) == 0 {
			return voiceSpeakDoneMsg{engine: eng, err: err}
		}
		if ctx.Err() != nil {
			return voiceSpeakDoneMsg{engine: eng}
		}
		playErr := m.voice.Player.PlayWAV(ctx, wav)
		return voiceSpeakDoneMsg{engine: eng, err: playErr}
	})
}

// startVoiceDemo runs a live formant-modulated synthetic speech signal through
// the player and DSP analyzer so the user can inspect the equalizer and waveform.
func (m *Model) startVoiceDemo() tea.Cmd {
	if m.voice == nil {
		return nil
	}
	if !m.Cfg.TUI.VoiceEnabled {
		m.Cfg.TUI.VoiceEnabled = true
	}
	m.stopVoiceSpeaking()
	ctx, cancel := context.WithCancel(context.Background())
	m.voice.SpeakCancel = cancel
	m.voice.State = VoiceSpeaking
	m.voice.ActiveEngine = "allan-formant-synth"
	m.setVoiceNote(i18n.S("тест визуализатора и аудиоканала"))
	wav := voice.SynthesizeDemoSpeechWAV(2600 * time.Millisecond)

	return tea.Batch(m.ensureVoiceTick(), func() tea.Msg {
		defer cancel()
		err := m.voice.Player.PlayWAV(ctx, wav)
		return voiceSpeakDoneMsg{engine: "allan-formant-synth", err: err}
	})
}

func (m *Model) handleVoiceMsg(msg tea.Msg) (tea.Cmd, bool) {
	if m.voice == nil {
		return nil, false
	}
	switch msg := msg.(type) {
	case voiceCatalogMsg:
		m.voice.Catalog = msg.items
		switch msg.openKind {
		case "stt":
			m.openAudioModelPicker("stt", false)
		case "realtime":
			m.openAudioModelPicker("stt", true)
		case "tts":
			m.openAudioModelPicker("tts", false)
		case "voice":
			m.openTTSVoicePicker()
		}
		return nil, true

	case voiceVisTickMsg:
		m.voice.SpinnerFrame = (m.voice.SpinnerFrame + 1) % len(transSpin)
		if m.voice.State == VoiceIdle || m.voice.State == VoiceTranscribing {
			m.voice.Analyzer.Decay()
		}
		st := m.voice.Analyzer.Snapshot()
		stillAnimating := m.voice.State != VoiceIdle || st.RMS > 0.008
		if stillAnimating {
			return voiceVisTickCmd(), true
		}
		m.voice.TickRunning = false
		return nil, true

	case voiceSpaceCheckMsg:
		if m.voice.State != VoiceRecording || msg.seq != m.voice.SpaceSeq {
			return nil, true
		}
		if msg.initial {
			if m.voice.CaptureMode == VoiceModeCandidate {
				m.voice.CaptureMode = VoiceModeVAD
				m.voice.Recorder.SetVAD(true)
			}
			return nil, true
		}
		if m.voice.CaptureMode == VoiceModePTT {
			return m.stopVoiceRecording(false), true
		}
		return nil, true

	case voiceVADAutoStopMsg:
		if m.voice.State == VoiceRecording {
			return m.stopVoiceRecording(false), true
		}
		return nil, true

	case voiceTranscribeResultMsg:
		m.voice.State = VoiceIdle
		m.voice.Analyzer.Reset()
		if msg.engine != "" {
			m.voice.ActiveEngine = msg.engine
		}
		if msg.err != nil {
			m.setVoiceNote(i18n.S("ошибка STT"))
			m.appendMsg(Message{Kind: MsgWarn, Text: i18n.S("Распознавание речи: ") + msg.err.Error()})
			return nil, true
		}
		text := strings.TrimSpace(msg.text)
		if text == "" {
			m.setVoiceNote(i18n.S("слов не разобрано"))
			return nil, true
		}
		m.setVoiceNote(i18n.S("распознано · ") + msg.engine)
		cur := strings.TrimSpace(m.textarea.Value())
		if cur != "" {
			text = cur + " " + text
		}
		if m.Cfg.TUI.VoiceAutoSend && !m.thinking {
			m.textarea.SetValue("")
			m.history = append(m.history, redactSensitiveInput(text))
			m.historyIdx = len(m.history)
			return m.submit(text), true
		}
		m.textarea.SetValue(text)
		m.textarea.CursorEnd()
		m.updatePopup()
		return nil, true

	case voiceSpeakDoneMsg:
		if m.voice.State == VoiceSpeaking {
			m.voice.State = VoiceIdle
		}
		if msg.engine != "" {
			m.voice.ActiveEngine = msg.engine
		}
		return m.ensureVoiceTick(), true
	}
	return nil, false
}

// openAudioModelPicker opens an interactive tabbed picker (Мои ПК / BYOK / Напрямую / Этот ПК)
// for selecting STT, TTS, or Realtime audio models.
func (m *Model) openAudioModelPicker(kind string, realtimeOnly bool) {
	var items []pickItem
	curID := m.Cfg.TUI.VoiceSTTModel
	if kind == "tts" {
		curID = m.Cfg.TUI.VoiceTTSModel
		items = append(items, pickItem{
			Label:   i18n.S("[Выключить озвучку TTS]"),
			Group:   i18n.S("Режим"),
			Detail:  i18n.S("не озвучивать ответы агента"),
			Current: !m.Cfg.TUI.VoiceTTS,
			Free:    true,
			Section: voice.SecMine,
			Value:   voice.AudioModelChoice{ID: "__off__", Kind: "tts"},
		})
	}
	items = append(items, pickItem{
		Label:   i18n.S("[Автовыбор: облако BYOK / Мои ПК → локально]"),
		Group:   i18n.S("Режим"),
		Detail:  i18n.S("не нагружает VRAM, если есть BYOK или другой ПК"),
		Current: curID == "" && (kind != "tts" || m.Cfg.TUI.VoiceTTS),
		Free:    true,
		Section: voice.SecMine,
		Value:   voice.AudioModelChoice{ID: "", Kind: kind},
	})

	for _, c := range m.voice.Catalog {
		if c.Kind != kind {
			continue
		}
		if realtimeOnly && !c.Realtime {
			continue
		}
		detail := c.Price
		if c.Realtime && !strings.Contains(strings.ToLower(detail), "realtime") && !strings.Contains(strings.ToLower(detail), "multimodal") {
			if detail != "" {
				detail += " · realtime"
			} else {
				detail = "realtime"
			}
		}
		if len(c.Voices) > 0 {
			detail += fmt.Sprintf(i18n.S(" · %d голос(ов)"), len(c.Voices))
		}
		grp := c.Group
		if c.Section < len(audioSectionTitles) && c.Section != voice.SecBYOK {
			secTitle := i18n.S(audioSectionTitles[c.Section])
			if !strings.HasPrefix(grp, secTitle) {
				grp = secTitle + " · " + grp
			}
		}
		items = append(items, pickItem{
			Label:   c.Label,
			Group:   grp,
			Detail:  detail,
			Current: curID == c.ID && (kind != "tts" || m.Cfg.TUI.VoiceTTS),
			Free:    c.Free,
			Section: c.Section,
			Value:   c,
		})
	}

	title := i18n.S("Модель распознавания речи (STT — BYOK, Мои ПК, Realtime, Локально)")
	if realtimeOnly {
		title = i18n.S("Realtime / Мультимодальные аудио-модели (прямой аудиоввод + живой режим)")
	} else if kind == "tts" {
		title = i18n.S("Модель синтеза речи (TTS — BYOK, Мои ПК, Локально)")
	}

	sections := make([]string, len(audioSectionTitles))
	for i, s := range audioSectionTitles {
		sections[i] = i18n.S(s)
	}

	p := newPicker(title, items, func(it pickItem) tea.Cmd {
		choice, ok := it.Value.(voice.AudioModelChoice)
		if !ok {
			return nil
		}
		if kind == "tts" {
			if choice.ID == "__off__" {
				m.Cfg.TUI.VoiceTTS = false
				m.stopVoiceSpeaking()
				_ = config.Save(m.Cfg)
				m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Голосовой вывод (TTS) выключен")})
				return nil
			}
			m.Cfg.TUI.VoiceTTS = true
			m.Cfg.TUI.VoiceTTSModel = choice.ID
			m.Cfg.TUI.VoiceTTSVoice = ""
			_ = config.Save(m.Cfg)
			if len(choice.Voices) > 1 {
				m.openTTSVoicePickerFor(choice)
				return nil
			}
			if len(choice.Voices) == 1 {
				m.Cfg.TUI.VoiceTTSVoice = choice.Voices[0].ID
				_ = config.Save(m.Cfg)
			}
			m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Модель озвучки (TTS): ") + m.preferredTTSEngineLabel()})
			return nil
		}

		m.Cfg.TUI.VoiceSTTModel = choice.ID
		if realtimeOnly {
			m.Cfg.TUI.VoiceAutoSend = true
			m.Cfg.TUI.VoiceTTS = true
		}
		_ = config.Save(m.Cfg)
		msg := i18n.S("Движок распознавания (STT): ") + m.preferredSTTEngineLabel()
		if realtimeOnly {
			msg += i18n.S(" · живой режим включён (автоотправка + озвучка)")
		}
		m.appendMsg(Message{Kind: MsgSys, Text: msg})
		return nil
	})
	p.setSections(sections)
	m.openPicker(p)
}

func (m *Model) openTTSVoicePicker() {
	curModel := m.Cfg.TUI.VoiceTTSModel
	var target *voice.AudioModelChoice
	for i := range m.voice.Catalog {
		c := &m.voice.Catalog[i]
		if c.Kind == "tts" && (c.ID == curModel || (curModel == "" && target == nil)) {
			target = c
			if c.ID == curModel {
				break
			}
		}
	}
	if target == nil || len(target.Voices) == 0 {
		m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("У текущей модели озвучки нет отдельных голосов для выбора")})
		return
	}
	m.openTTSVoicePickerFor(*target)
}

func (m *Model) openTTSVoicePickerFor(choice voice.AudioModelChoice) {
	items := make([]pickItem, 0, len(choice.Voices))
	for _, v := range choice.Voices {
		items = append(items, pickItem{
			Label:   v.Name,
			Group:   choice.Label,
			Detail:  v.ID,
			Current: m.Cfg.TUI.VoiceTTSVoice == v.ID,
			Free:    choice.Free,
			Value:   v.ID,
		})
	}
	p := newPicker(i18n.S("Выберите голос для ")+choice.Label, items, func(it pickItem) tea.Cmd {
		vid, _ := it.Value.(string)
		m.Cfg.TUI.VoiceTTSVoice = vid
		_ = config.Save(m.Cfg)
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("Модель озвучки (TTS): %s · голос %s"), m.preferredTTSEngineLabel(), vid)})
		return nil
	})
	m.openPicker(p)
}

// renderVoiceDock renders the 2-line Voice Comms & Visualizer panel directly
// below the input box.
func (m *Model) renderVoiceDock() string {
	if m.voice == nil || !m.Cfg.TUI.VoiceEnabled {
		return ""
	}
	st := m.voice.Analyzer.Snapshot()
	w := maxInt(24, m.width-2)

	// 1. State badge
	var badge string
	switch m.voice.State {
	case VoiceRecording:
		modeTag := "PTT"
		if m.voice.CaptureMode == VoiceModeVAD {
			modeTag = "VAD"
		}
		badge = StyleError.Bold(true).Render("● REC") + " " + StyleWarn.Render(modeTag)
	case VoiceTranscribing:
		frame := transSpin[m.voice.SpinnerFrame%len(transSpin)]
		badge = StyleWarn.Bold(true).Render(frame + " STT")
	case VoiceSpeaking:
		badge = StyleOK.Bold(true).Render("◉ TTS")
	default:
		badge = StyleKey.Bold(true).Render("◎ VOICE")
	}

	// 2. 16-band spectrum analyzer with peak coloring
	var spec strings.Builder
	spec.WriteString(StyleDim.Render("▐"))
	for b := 0; b < voice.NumBands; b++ {
		val := st.Bands[b]
		peak := st.BandPeaks[b]
		if m.voice.State == VoiceIdle && val < 0.04 {
			spec.WriteString(StyleDim.Render("▁"))
			continue
		}
		idx := int(val * float64(len(spectrumBars)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(spectrumBars) {
			idx = len(spectrumBars) - 1
		}
		ch := string(spectrumBars[idx])
		switch {
		case val >= 0.78 || peak >= 0.90:
			spec.WriteString(StyleWarn.Render(ch))
		case val >= 0.38:
			spec.WriteString(StyleOK.Render(ch))
		case val >= 0.10:
			spec.WriteString(StyleKey.Render(ch))
		default:
			spec.WriteString(StyleDim.Render(ch))
		}
	}
	spec.WriteString(StyleDim.Render("▌"))

	// 3. Scrolling Braille waveform history
	waveCells := 18
	if w < 96 {
		waveCells = 10
	}
	if w < 68 {
		waveCells = 0
	}
	var wave strings.Builder
	if waveCells > 0 {
		wave.WriteString(" ")
		start := voice.HistoryLen - waveCells
		if start < 0 {
			start = 0
		}
		for i := start; i < voice.HistoryLen; i++ {
			v := st.History[i]
			if m.voice.State == VoiceIdle && v < 0.03 {
				wave.WriteString(StyleDim.Render("⠤"))
				continue
			}
			idx := int(v * float64(len(brailleWave)-1))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(brailleWave) {
				idx = len(brailleWave) - 1
			}
			ch := string(brailleWave[idx])
			if m.voice.State == VoiceSpeaking {
				wave.WriteString(StyleOK.Render(ch))
			} else if v >= 0.45 {
				wave.WriteString(StyleKey.Render(ch))
			} else {
				wave.WriteString(StyleMuted.Render(ch))
			}
		}
	}

	// 4. Right telemetry: level in dBFS + timer or active Cloud BYOK / Remote PC engine
	var tele strings.Builder
	switch m.voice.State {
	case VoiceRecording:
		dur := m.voice.Recorder.Elapsed()
		tele.WriteString(StyleText.Render(fmt.Sprintf("%02d:%02d.%d",
			int(dur.Minutes()), int(dur.Seconds())%60, int(dur.Milliseconds()/100)%10)))
		tele.WriteString(StyleDim.Render(fmt.Sprintf(" · %+3.0f dB", st.DBFS)))
	case VoiceTranscribing:
		tele.WriteString(StyleWarn.Render(i18n.S("распознаю · ") + truncate(m.preferredSTTEngineLabel(), 28)))
	case VoiceSpeaking:
		eng := m.voice.ActiveEngine
		if eng == "" {
			eng = m.preferredTTSEngineLabel()
		}
		tele.WriteString(StyleOK.Render(truncate(eng, 28)))
		tele.WriteString(StyleDim.Render(fmt.Sprintf(" · %+3.0f dB", st.DBFS)))
	default:
		sttLabel := truncate(m.preferredSTTEngineLabel(), 26)
		ttsPart := "TTS:off"
		if m.Cfg.TUI.VoiceTTS {
			ttsPart = "🔊 " + truncate(m.preferredTTSEngineLabel(), 20)
		}
		autoFlag := ""
		if m.Cfg.TUI.VoiceAutoSend {
			autoFlag = " · live"
		}
		tele.WriteString(StyleMuted.Render(sttLabel))
		tele.WriteString(StyleDim.Render(" · " + ttsPart + autoFlag))
	}

	leftPart := " " + badge + " " + spec.String() + wave.String()
	rightPart := tele.String() + " "
	gap := maxInt(1, m.width-lipgloss.Width(leftPart)-lipgloss.Width(rightPart))
	line1 := leftPart + strings.Repeat(" ", gap) + rightPart

	// Line 2: contextual control hint & transient status note under the visualizer
	var hint string
	switch m.voice.State {
	case VoiceRecording:
		if m.voice.CaptureMode == VoiceModePTT {
			hint = i18n.S(" 🎙 Удерживайте Пробел и говорите (отпустите — распознать · Esc — отмена)")
		} else {
			hint = i18n.S(" 🎙 Слушаю… говорите (автостоп по паузе · Пробел/Enter — готово · Esc — отмена)")
		}
	case VoiceTranscribing:
		hint = i18n.S(" ◌ Перевожу голос в текст…")
	case VoiceSpeaking:
		hint = i18n.S(" 🔊 Озвучиваю ответ агента (Esc или /voice stop — остановить)")
	default:
		if voice.CanRecord() {
			hint = i18n.S(" 🎙 Зажать Пробел — говорить · Ctrl+G — запись · /voice stt|tts|realtime — выбор BYOK/ПК")
		} else {
			hint = i18n.S(" 🔊 Ответы озвучиваются · голосовой ввод в Termux недоступен · /voice tts — выбор голоса")
		}
		if m.voice.StatusNote != "" && time.Since(m.voice.StatusNoteAt) < 8*time.Second {
			hint += " · " + m.voice.StatusNote
		}
	}
	if lipgloss.Width(hint) > m.width {
		hint = truncate(hint, maxInt(10, m.width-1))
	}
	line2 := StyleDim.Render(hint)
	return line1 + "\n" + line2
}

func (m *Model) preferredSTTEngineLabel() string {
	if m.Cfg.TUI.VoiceSTTModel != "" {
		return voice.ShortModelLabel(m.Cfg.TUI.VoiceSTTModel)
	}
	if m.voice != nil {
		// то же, что авторежим попробует первым (сработавшая модель или первый кандидат)
		if c := voice.AutoOrder(m.voice.Catalog, "stt"); len(c) > 0 {
			return voice.ShortModelLabel(c[0].ID)
		}
		for _, c := range m.voice.Catalog {
			if c.Kind == "stt" {
				return voice.ShortModelLabel(c.ID)
			}
		}
	}
	return i18n.S("авто (облако/ПК)")
}

func (m *Model) preferredTTSEngineLabel() string {
	if m.Cfg.TUI.VoiceTTSModel != "" {
		lbl := voice.ShortModelLabel(m.Cfg.TUI.VoiceTTSModel)
		if m.Cfg.TUI.VoiceTTSVoice != "" {
			lbl += ":" + m.Cfg.TUI.VoiceTTSVoice
		}
		return lbl
	}
	if m.voice != nil {
		if c := voice.AutoOrder(m.voice.Catalog, "tts"); len(c) > 0 {
			return voice.ShortModelLabel(c[0].ID)
		}
		for _, c := range m.voice.Catalog {
			if c.Kind == "tts" {
				return voice.ShortModelLabel(c.ID)
			}
		}
	}
	return i18n.S("авто (облако/ПК)")
}

// handleVoiceSlash processes `/voice [stt|tts|voice|realtime|refresh|on|off|auto|record|test|stop|status]`.
func (m *Model) handleVoiceSlash(args []string) tea.Cmd {
	if len(args) == 0 {
		// Opening `/voice` with no arguments opens the STT/Realtime/TTS summary + refreshes catalog if empty
		m.appendMsg(Message{Kind: MsgSys, Text: m.voiceStatusSummary()})
		if m.voice != nil && len(m.voice.Catalog) == 0 {
			return m.refreshVoiceCatalog(false, "")
		}
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "status":
		m.appendMsg(Message{Kind: MsgSys, Text: m.voiceStatusSummary()})
	case "refresh":
		m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Обновляю список голосовых моделей (BYOK, Мои ПК, Realtime)…")})
		return m.refreshVoiceCatalog(true, "stt")
	case "stt":
		if len(args) == 1 || args[1] == "pick" {
			if m.voice != nil && len(m.voice.Catalog) > 0 {
				m.openAudioModelPicker("stt", false)
				return nil
			}
			return m.refreshVoiceCatalog(false, "stt")
		}
		if args[1] == "auto" || args[1] == "default" {
			m.Cfg.TUI.VoiceSTTModel = ""
		} else {
			m.Cfg.TUI.VoiceSTTModel = args[1]
		}
		_ = config.Save(m.Cfg)
		m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Движок распознавания (STT): ") + m.preferredSTTEngineLabel()})
	case "realtime", "live":
		if m.voice != nil && len(m.voice.Catalog) > 0 {
			m.openAudioModelPicker("stt", true)
			return nil
		}
		return m.refreshVoiceCatalog(false, "realtime")
	case "tts":
		if len(args) == 1 || args[1] == "pick" {
			if m.voice != nil && len(m.voice.Catalog) > 0 {
				m.openAudioModelPicker("tts", false)
				return nil
			}
			return m.refreshVoiceCatalog(false, "tts")
		}
		switch strings.ToLower(args[1]) {
		case "on":
			m.Cfg.TUI.VoiceTTS = true
		case "off":
			m.Cfg.TUI.VoiceTTS = false
			m.stopVoiceSpeaking()
		case "auto", "default":
			m.Cfg.TUI.VoiceTTS = true
			m.Cfg.TUI.VoiceTTSModel = ""
			m.Cfg.TUI.VoiceTTSVoice = ""
		default:
			m.Cfg.TUI.VoiceTTSModel = args[1]
			m.Cfg.TUI.VoiceTTS = true
		}
		_ = config.Save(m.Cfg)
		if m.Cfg.TUI.VoiceTTS {
			m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Голосовой вывод (TTS) включён · ") + m.preferredTTSEngineLabel()})
		} else {
			m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Голосовой вывод (TTS) выключен")})
		}
	case "voice":
		if len(args) >= 2 {
			m.Cfg.TUI.VoiceTTSVoice = args[1]
			_ = config.Save(m.Cfg)
			m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("Модель озвучки (TTS): %s · голос %s"), m.preferredTTSEngineLabel(), args[1])})
			return nil
		}
		if m.voice != nil && len(m.voice.Catalog) > 0 {
			m.openTTSVoicePicker()
			return nil
		}
		return m.refreshVoiceCatalog(false, "voice")
	case "on":
		m.Cfg.TUI.VoiceEnabled = true
		_ = config.Save(m.Cfg)
		m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Голосовой модуль и визуализатор включены")})
	case "off":
		if m.voice != nil && m.voice.State == VoiceRecording {
			_ = m.stopVoiceRecording(true)
		}
		m.stopVoiceSpeaking()
		m.Cfg.TUI.VoiceEnabled = false
		_ = config.Save(m.Cfg)
		m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Голосовой модуль скрыт (/voice on — вернуть)")})
	case "lang":
		if len(args) >= 2 {
			v := strings.ToLower(strings.TrimSpace(args[1]))
			if v == "system" || v == "default" {
				v = ""
			}
			m.Cfg.TUI.VoiceLang = v
			_ = config.Save(m.Cfg)
		}
		cur := voice.SpeechLang(m.Cfg.TUI.VoiceLang)
		if cur == "" {
			cur = "auto"
		}
		m.appendMsg(Message{Kind: MsgSys, Text: fmt.Sprintf(i18n.S("Язык речи: %s (/voice lang ru|en|de|auto|system)"), cur)})
		return nil
	case "auto":
		if len(args) >= 2 {
			m.Cfg.TUI.VoiceAutoSend = strings.ToLower(args[1]) == "on"
		} else {
			m.Cfg.TUI.VoiceAutoSend = !m.Cfg.TUI.VoiceAutoSend
		}
		_ = config.Save(m.Cfg)
		if m.Cfg.TUI.VoiceAutoSend {
			m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Автоотправка распознанной речи включена")})
		} else {
			m.appendMsg(Message{Kind: MsgSys, Text: i18n.S("Автоотправка выключена: распознанный текст помещается в поле ввода")})
		}
	case "record", "listen", "rec":
		if m.voice != nil && m.voice.State == VoiceRecording {
			return m.stopVoiceRecording(false)
		}
		return m.startVoiceRecording(VoiceModeVAD)
	case "test", "demo":
		return m.startVoiceDemo()
	case "stop":
		if m.voice != nil && m.voice.State == VoiceRecording {
			return m.stopVoiceRecording(true)
		}
		m.stopVoiceSpeaking()
		m.setVoiceNote(i18n.S("остановлено"))
	default:
		m.appendMsg(Message{Kind: MsgWarn, Text: i18n.S("Использование: /voice [stt|tts|voice|lang|realtime|refresh|auto|record|test|stop|on|off]")})
	}
	m.recalcLayout()
	return nil
}

func (m *Model) voiceStatusSummary() string {
	var sb strings.Builder
	sb.WriteString(i18n.S("Голосовой модуль связи Allan\n"))
	panelState := i18n.S("включён")
	if !m.Cfg.TUI.VoiceEnabled {
		panelState = i18n.S("выключен")
	}
	ttsState := i18n.S("включён")
	if !m.Cfg.TUI.VoiceTTS {
		ttsState = i18n.S("выключен")
	}
	autoState := i18n.S("выключена (текст в поле ввода)")
	if m.Cfg.TUI.VoiceAutoSend {
		autoState = i18n.S("включена (сразу отправляет агенту)")
	}
	cloudSTT, cloudTTS, peerCount := 0, 0, 0
	if m.voice != nil {
		for _, c := range m.voice.Catalog {
			if c.Section == voice.SecMine {
				peerCount++
			} else if c.Section != voice.SecLocal {
				if c.Kind == "stt" {
					cloudSTT++
				} else {
					cloudTTS++
				}
			}
		}
	}
	sb.WriteString(fmt.Sprintf(i18n.S("  панель внизу   %s (/voice on|off)\n"), panelState))
	if voice.CanRecord() {
		sb.WriteString(fmt.Sprintf(i18n.S("  ввод (STT)     %s (/voice stt — список BYOK / Мои ПК / Realtime)\n"), m.preferredSTTEngineLabel()))
	} else {
		sb.WriteString(i18n.S("  ввод (STT)     недоступен в Termux: нет доступа к микрофону\n"))
	}
	sb.WriteString(fmt.Sprintf(i18n.S("  вывод (TTS)    %s · %s (/voice tts — список моделей, /voice voice — голос)\n"), ttsState, m.preferredTTSEngineLabel()))
	sb.WriteString(fmt.Sprintf(i18n.S("  каталог сети   облако BYOK: %d STT / %d TTS · другие ПК: %d (/voice refresh)\n"), cloudSTT, cloudTTS, peerCount))
	sb.WriteString(fmt.Sprintf(i18n.S("  автоотправка   %s (/voice auto)\n"), autoState))
	sb.WriteString(i18n.S("  управление     зажать Пробел — Push-to-Talk (в непустом поле допишет); нажать Пробел на пустом вводе или Ctrl+G — запись с автостопом; /voice realtime — живые модели"))
	return sb.String()
}

func (m *Model) voiceCompletions(input string) []SlashCommand {
	endsWithSpace := strings.HasSuffix(input, " ")
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 || (len(parts) == 2 && !endsWithSpace) {
		prefix := ""
		if len(parts) == 2 {
			prefix = parts[1]
		}
		return filterCompletions(prefix, []SlashCommand{
			{Name: "stt", Help: i18n.S("выбрать модель распознавания (BYOK, Мои ПК, Realtime, Локально)"), Insert: "/voice stt"},
			{Name: "tts", Help: i18n.S("выбрать модель синтеза речи (BYOK, Мои ПК, Локально) или вкл/выкл"), Insert: "/voice tts"},
			{Name: "voice", Help: i18n.S("выбрать голос текущей TTS-модели (alloy, Марфа, ElevenLabs…)"), Insert: "/voice voice"},
			{Name: "realtime", Help: i18n.S("выбрать мультимодальную Realtime аудио-модель и живой режим"), Insert: "/voice realtime"},
			{Name: "refresh", Help: i18n.S("обновить каталог голосовых моделей с KEYTRON Prime и других ПК"), Insert: "/voice refresh"},
			{Name: "test", Help: i18n.S("демо-сигнал для проверки спектра и эквалайзера"), Insert: "/voice test"},
			{Name: "record", Help: i18n.S("начать или остановить запись голоса с микрофона"), Insert: "/voice record"},
			{Name: "auto", Help: i18n.S("переключить автоотправку после распознавания"), Insert: "/voice auto"},
			{Name: "stop", Help: i18n.S("остановить текущую запись или озвучку"), Insert: "/voice stop"},
			{Name: "on", Help: i18n.S("показать панель связи и визуализатор под полем ввода"), Insert: "/voice on"},
			{Name: "off", Help: i18n.S("скрыть панель связи под полем ввода"), Insert: "/voice off"},
			{Name: "status", Help: i18n.S("показать состояние голосового модуля"), Insert: "/voice status"},
		})
	}
	return nil
}
