package voice

import (
	"encoding/binary"
	"math"
	"sync"
)

const (
	// SampleRate is the internal capture and analysis PCM sample rate (16 kHz).
	SampleRate = 16000
	// NumBands is the number of frequency bands in the spectrum equalizer.
	NumBands = 16
	// HistoryLen is the number of recent amplitude frames kept for the scrolling waveform.
	HistoryLen = 32
)

// Center frequencies (Hz) for the 16-band speech spectrum analyzer (110 Hz .. 6000 Hz).
var bandFreqs = [NumBands]float64{
	110, 150, 210, 290, 400, 550, 750, 1000,
	1350, 1800, 2350, 3000, 3750, 4500, 5250, 6000,
}

// Per-band perceptual gain compensation so mid/high speech formants look balanced
// alongside fundamental vocal frequencies.
var bandGains = [NumBands]float64{
	1.1, 1.15, 1.2, 1.3, 1.45, 1.65, 1.85, 2.1,
	2.4, 2.75, 3.1, 3.5, 3.9, 4.3, 4.7, 5.1,
}

// FrameStats holds an immutable snapshot of the analyzer's state for TUI rendering.
type FrameStats struct {
	RMS       float64             // Normalized RMS amplitude [0..1]
	Peak      float64             // Normalized peak amplitude [0..1]
	DBFS      float64             // Decibels relative to full scale [-60..0]
	Bands     [NumBands]float64   // Smoothed spectrum bands [0..1]
	BandPeaks [NumBands]float64   // Peak-hold markers per band [0..1]
	History   [HistoryLen]float64 // Scrolling envelope history [0..1]
	Active    bool                // True when signal exceeds noise gate
}

// Analyzer processes real-time 16-bit signed little-endian PCM streams and
// maintains smoothed spectrum and waveform state for the terminal visualizer.
type Analyzer struct {
	mu          sync.Mutex
	rms         float64
	peak        float64
	dbfs        float64
	bands       [NumBands]float64
	bandPeaks   [NumBands]float64
	history     [HistoryLen]float64
	active      bool
	noiseFloor  float64
	initialized bool
}

// NewAnalyzer returns a ready-to-use audio DSP analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		dbfs:       -60.0,
		noiseFloor: 0.004,
	}
}

// Reset clears the current spectrum and waveform back to silence.
func (a *Analyzer) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rms = 0
	a.peak = 0
	a.dbfs = -60.0
	a.bands = [NumBands]float64{}
	a.bandPeaks = [NumBands]float64{}
	a.history = [HistoryLen]float64{}
	a.active = false
}

// Decay steps the visualizer smoothly down toward zero when no audio frame arrived.
func (a *Analyzer) Decay() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rms *= 0.78
	a.peak *= 0.82
	if a.rms < 1e-4 {
		a.dbfs = -60.0
	} else {
		db := 20 * math.Log10(a.rms)
		if db < -60 {
			db = -60
		}
		a.dbfs = db
	}
	for i := 0; i < NumBands; i++ {
		a.bands[i] *= 0.76
		a.bandPeaks[i] *= 0.90
		if a.bandPeaks[i] < a.bands[i] {
			a.bandPeaks[i] = a.bands[i]
		}
	}
	copy(a.history[:HistoryLen-1], a.history[1:])
	a.history[HistoryLen-1] = a.rms
	a.active = a.rms > 0.015
}

// PushPCM16LE ingests a raw s16le mono PCM byte slice (at SampleRate) and updates
// the spectrum, RMS, dBFS, and waveform history.
func (a *Analyzer) PushPCM16LE(pcm []byte) FrameStats {
	n := len(pcm) / 2
	if n == 0 {
		a.Decay()
		return a.Snapshot()
	}
	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		samples[i] = float64(v) / 32768.0
	}
	return a.PushSamples(samples)
}

// PushSamples ingests normalized float64 audio samples in [-1..1] and updates
// the analyzer state.
func (a *Analyzer) PushSamples(samples []float64) FrameStats {
	n := len(samples)
	if n == 0 {
		a.Decay()
		return a.Snapshot()
	}

	var sumSq, maxAbs float64
	for _, s := range samples {
		sumSq += s * s
		if abs := math.Abs(s); abs > maxAbs {
			maxAbs = abs
		}
	}
	rawRMS := math.Sqrt(sumSq / float64(n))

	// Pre-emphasis + Hann window on up to 512 samples for Goertzel spectrum analysis.
	winN := n
	if winN > 512 {
		winN = 512
	}
	windowed := make([]float64, winN)
	var prev float64
	for i := 0; i < winN; i++ {
		s := samples[i] - 0.92*prev
		prev = samples[i]
		hann := 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(max(1, winN-1))))
		windowed[i] = s * hann
	}

	var rawBands [NumBands]float64
	invN := 2.0 / float64(winN)
	subRatios := [3]float64{0.89, 1.0, 1.12}
	for b := 0; b < NumBands; b++ {
		var maxMag float64
		for _, r := range subRatios {
			freq := bandFreqs[b] * r
			if freq >= float64(SampleRate)/2-100 {
				freq = float64(SampleRate)/2 - 100
			}
			omega := 2.0 * math.Pi * freq / float64(SampleRate)
			coeff := 2.0 * math.Cos(omega)
			var q1, q2 float64
			for _, x := range windowed {
				q0 := coeff*q1 - q2 + x
				q2 = q1
				q1 = q0
			}
			power := q1*q1 + q2*q2 - q1*q2*coeff
			if power < 0 {
				power = 0
			}
			if mag := math.Sqrt(power) * invN; mag > maxMag {
				maxMag = mag
			}
		}
		mag := maxMag * bandGains[b]
		// Perceptual compression (log/sqrt curve) so quiet and loud speech both animate clearly.
		level := math.Pow(clamp01(mag*4.8), 0.58)
		rawBands[b] = clamp01(level)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.initialized {
		a.noiseFloor = math.Max(0.002, math.Min(0.02, rawRMS))
		a.initialized = true
	} else if rawRMS < a.noiseFloor*1.5 {
		a.noiseFloor = 0.96*a.noiseFloor + 0.04*rawRMS
	}

	// Visual envelope scaled for natural speech levels (typical speech RMS is 0.03..0.25).
	visEnv := clamp01(math.Pow(clamp01(rawRMS*4.2), 0.65))
	if visEnv > a.rms {
		a.rms = 0.35*a.rms + 0.65*visEnv
	} else {
		a.rms = 0.72*a.rms + 0.28*visEnv
	}
	if maxAbs > a.peak {
		a.peak = maxAbs
	} else {
		a.peak = 0.80*a.peak + 0.20*maxAbs
	}

	if rawRMS < 1e-4 {
		a.dbfs = -60.0
	} else {
		db := 20.0 * math.Log10(rawRMS)
		if db < -60.0 {
			db = -60.0
		}
		if db > 0.0 {
			db = 0.0
		}
		a.dbfs = 0.6*a.dbfs + 0.4*db
	}

	for b := 0; b < NumBands; b++ {
		target := rawBands[b]
		if target > a.bands[b] {
			a.bands[b] = 0.30*a.bands[b] + 0.70*target // fast attack
		} else {
			a.bands[b] = 0.74*a.bands[b] + 0.26*target // smooth release
		}
		if a.bands[b] >= a.bandPeaks[b] {
			a.bandPeaks[b] = a.bands[b]
		} else {
			a.bandPeaks[b] = math.Max(a.bands[b], a.bandPeaks[b]*0.91)
		}
	}

	copy(a.history[:HistoryLen-1], a.history[1:])
	a.history[HistoryLen-1] = a.rms
	a.active = rawRMS > math.Max(0.010, a.noiseFloor*2.6)

	return a.snapshotLocked()
}

// Snapshot returns a thread-safe copy of the current analyzer stats.
func (a *Analyzer) Snapshot() FrameStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

func (a *Analyzer) snapshotLocked() FrameStats {
	return FrameStats{
		RMS:       a.rms,
		Peak:      a.peak,
		DBFS:      a.dbfs,
		Bands:     a.bands,
		BandPeaks: a.bandPeaks,
		History:   a.history,
		Active:    a.active,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
