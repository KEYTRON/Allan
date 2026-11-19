package voice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// CaptureFrameBytes is 30 ms of 16 kHz mono s16le PCM (480 samples * 2 bytes = 960 bytes).
const CaptureFrameBytes = 960

// Recorder streams live microphone PCM into an Analyzer, accumulates the WAV
// buffer, and optionally triggers an auto-stop callback when VAD detects end-of-speech.
type Recorder struct {
	analyzer *Analyzer

	mu         sync.Mutex
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	pcmBuf     bytes.Buffer
	recording  bool
	vadEnabled bool
	speechSeen bool
	loudFrames int
	silentFor  time.Duration
	startedAt  time.Time
	onAutoStop func()
	doneCh     chan struct{}
}

// NewRecorder creates a microphone recorder coupled to the given DSP Analyzer.
func NewRecorder(analyzer *Analyzer) *Recorder {
	return &Recorder{analyzer: analyzer}
}

// Start begins capturing 16 kHz mono s16le audio from the default microphone.
// If vadAutoStop is true, onAutoStop is invoked automatically after ~1.3s of
// silence following detected speech.
func (r *Recorder) Start(vadAutoStop bool, onAutoStop func()) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recording {
		return nil
	}
	name, args, err := captureCommand()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	r.cmd = cmd
	r.cancel = cancel
	r.pcmBuf.Reset()
	r.recording = true
	r.vadEnabled = vadAutoStop
	r.speechSeen = false
	r.loudFrames = 0
	r.silentFor = 0
	r.startedAt = time.Now()
	r.onAutoStop = onAutoStop
	doneCh := make(chan struct{})
	r.doneCh = doneCh

	go r.readLoop(stdout, doneCh)
	return nil
}

// SetVAD switches VAD auto-stop on or off while recording (used when a Space
// press transitions from potential tap-VAD to hold-to-talk).
func (r *Recorder) SetVAD(enabled bool) {
	r.mu.Lock()
	r.vadEnabled = enabled
	r.mu.Unlock()
}

// IsRecording reports whether microphone capture is currently active.
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Elapsed returns how long the current recording has been running.
func (r *Recorder) Elapsed() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording || r.startedAt.IsZero() {
		return 0
	}
	return time.Since(r.startedAt)
}

// Stop terminates microphone capture and returns the recorded 16 kHz mono WAV
// bytes along with whether intelligible speech energy was observed.
func (r *Recorder) Stop() (wav []byte, hasSpeech bool) {
	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return nil, false
	}
	r.recording = false
	cancel := r.cancel
	cmd := r.cmd
	doneCh := r.doneCh
	r.cancel = nil
	r.cmd = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(400 * time.Millisecond):
		}
	}

	r.mu.Lock()
	raw := append([]byte(nil), r.pcmBuf.Bytes()...)
	// только реально услышанная речь (≥120 мс над шумовым порогом): тишину
	// не отправляем на распознавание, иначе пустое нажатие стоит денег
	seen := r.speechSeen
	r.pcmBuf.Reset()
	r.mu.Unlock()

	if len(raw) < CaptureFrameBytes*3 {
		return nil, false
	}
	return EncodeWAV16kMono(raw, SampleRate), seen
}

func (r *Recorder) readLoop(stdout io.ReadCloser, doneCh chan struct{}) {
	defer close(doneCh)
	buf := make([]byte, CaptureFrameBytes)
	for {
		n, err := io.ReadFull(stdout, buf)
		if n > 0 {
			chunk := buf[:n-(n%2)]
			stats := r.analyzer.PushPCM16LE(chunk)

			r.mu.Lock()
			if !r.recording {
				r.mu.Unlock()
				return
			}
			r.pcmBuf.Write(chunk)
			if stats.Active {
				r.loudFrames++
				r.silentFor = 0
				if r.loudFrames >= 7 { // >= 210 ms подряд: щелчок клавиши (50–100 мс) речью не считается
					r.speechSeen = true
				}
			} else if r.speechSeen {
				r.silentFor += 30 * time.Millisecond
			} else {
				r.loudFrames = 0
			}
			shouldAutoStop := r.vadEnabled &&
				((r.speechSeen && r.silentFor >= 1300*time.Millisecond) ||
					time.Since(r.startedAt) >= 45*time.Second)
			cb := r.onAutoStop
			if shouldAutoStop {
				r.vadEnabled = false
			}
			r.mu.Unlock()

			if shouldAutoStop && cb != nil {
				go cb()
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func captureCommand() (string, []string, error) {
	if bin, err := exec.LookPath("parec"); err == nil {
		return bin, []string{
			"--format=s16le",
			"--rate=16000",
			"--channels=1",
			"--latency-msec=30",
		}, nil
	}
	if bin, err := exec.LookPath("pw-record"); err == nil {
		return bin, []string{
			"--format=s16",
			"--rate=16000",
			"--channels=1",
			"-",
		}, nil
	}
	if bin, err := exec.LookPath("arecord"); err == nil {
		return bin, []string{
			"-q",
			"-f", "S16_LE",
			"-r", "16000",
			"-c", "1",
			"-t", "raw",
		}, nil
	}
	if bin, err := exec.LookPath("sox"); err == nil {
		return bin, []string{
			"-q",
			"-d",
			"-r", "16000",
			"-c", "1",
			"-b", "16",
			"-e", "signed-integer",
			"-t", "raw",
			"-",
		}, nil
	}
	if bin, err := exec.LookPath("ffmpeg"); err == nil {
		inFmt := "pulse"
		if runtime.GOOS == "darwin" {
			inFmt = "avfoundation"
		}
		return bin, []string{
			"-loglevel", "error",
			"-f", inFmt,
			"-i", "default",
			"-ac", "1",
			"-ar", "16000",
			"-f", "s16le",
			"-",
		}, nil
	}
	return "", nil, fmt.Errorf("не найдена утилита записи звука (parec, pw-record, arecord, sox или ffmpeg)")
}

// Player plays synthesized WAV audio while feeding synchronized 30ms PCM frames
// into the Analyzer so the TUI spectrum/waveform dances to the agent's voice.
type Player struct {
	analyzer *Analyzer

	mu      sync.Mutex
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	playing bool
}

// NewPlayer creates an audio player coupled to the DSP Analyzer.
func NewPlayer(analyzer *Analyzer) *Player {
	return &Player{analyzer: analyzer}
}

// IsPlaying reports whether TTS audio is currently playing.
func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// Stop interrupts any active audio playback immediately.
func (p *Player) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	cmd := p.cmd
	p.playing = false
	p.cancel = nil
	p.cmd = nil
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// PlayWAV plays the given WAV audio buffer and synchronously feeds its 16 kHz
// PCM frames to the Analyzer at 30ms intervals until playback finishes or is stopped.
func (p *Player) PlayWAV(parent context.Context, wav []byte) error {
	p.Stop()

	pcm, _ := DecodeWAVTo16kMono(wav)
	tmp, err := os.CreateTemp("", "allan-voice-out-*.wav")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, _ = tmp.Write(wav)
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithCancel(parent)
	name, args := playbackCommand(tmpPath)
	var cmd *exec.Cmd
	if name != "" {
		cmd = exec.CommandContext(ctx, name, args...)
		_ = cmd.Start()
	}

	p.mu.Lock()
	p.cancel = cancel
	p.cmd = cmd
	p.playing = true
	p.mu.Unlock()

	defer func() {
		cancel()
		p.mu.Lock()
		p.playing = false
		p.cmd = nil
		p.cancel = nil
		p.mu.Unlock()
	}()

	cmdDone := make(chan struct{})
	if cmd != nil {
		go func() {
			_ = cmd.Wait()
			close(cmdDone)
		}()
	} else {
		close(cmdDone)
	}

	// Stream 30ms PCM slices into the visualizer in real time while the player runs.
	if len(pcm) > 0 {
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()
		offset := 0
		for offset < len(pcm) {
			end := offset + CaptureFrameBytes
			if end > len(pcm) {
				end = len(pcm)
			}
			p.analyzer.PushPCM16LE(pcm[offset:end])
			offset = end

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-cmdDone:
				if name != "" {
					return nil
				}
			case <-ticker.C:
			}
		}
	}

	if name != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cmdDone:
		}
	}
	return nil
}

func playbackCommand(wavPath string) (string, []string) {
	if runtime.GOOS == "darwin" {
		if bin, err := exec.LookPath("afplay"); err == nil {
			return bin, []string{wavPath}
		}
	}
	if bin, err := exec.LookPath("paplay"); err == nil {
		return bin, []string{wavPath}
	}
	if bin, err := exec.LookPath("pw-play"); err == nil {
		return bin, []string{wavPath}
	}
	if bin, err := exec.LookPath("mpv"); err == nil {
		return bin, []string{"--no-video", "--really-quiet", wavPath}
	}
	if bin, err := exec.LookPath("ffplay"); err == nil {
		return bin, []string{"-nodisp", "-autoexit", "-loglevel", "quiet", wavPath}
	}
	if bin, err := exec.LookPath("aplay"); err == nil {
		return bin, []string{"-q", wavPath}
	}
	return "", nil
}
