package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/link"
	"github.com/keytron/allan/agent/internal/serve"
)

// AudioSection groups voice models the same way /model groups chat models.
const (
	SecMine   = iota // Other computers of the user (via KEYTRON tunnel or Tailscale/LAN)
	SecBYOK          // Cloud BYOK models routed via KEYTRON Prime (/api/ai/v1/audio/*)
	SecDirect        // Direct cloud provider API keys configured in Allan
	SecLocal         // Local speech engines on this PC (uses local CPU/GPU)
)

// VoiceOption represents a selectable voice inside a TTS model (e.g. alloy, May_24000).
type VoiceOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AudioModelChoice describes a single STT, TTS, or Realtime/Multimodal voice model.
type AudioModelChoice struct {
	ID       string        // Canonical ID stored in config (e.g. "keytron:groq/whisper-large-v3-turbo", "direct:openai/gpt-4o-mini-tts", "peer:mac/apple-dictation", "local:whisper-small")
	Kind     string        // "stt" or "tts"
	Label    string        // Short display name
	Group    string        // Provider or machine group header
	Provider string        // Underlying provider slug
	Price    string        // Human-readable price or hardware note ("$0.0007/мин", "бесплатно", "своё железо", "локально · CPU/VRAM")
	Free     bool          // True if free or user's own hardware
	Realtime bool          // True if multimodal / realtime audio-capable LLM
	Section  int           // SecMine, SecBYOK, SecDirect, SecLocal
	Voices   []VoiceOption // Available voices for TTS models
}

var (
	openAIVoices = []VoiceOption{
		{ID: "alloy", Name: "alloy"},
		{ID: "ash", Name: "ash"},
		{ID: "ballad", Name: "ballad"},
		{ID: "coral", Name: "coral"},
		{ID: "echo", Name: "echo"},
		{ID: "fable", Name: "fable"},
		{ID: "nova", Name: "nova"},
		{ID: "onyx", Name: "onyx"},
		{ID: "sage", Name: "sage"},
		{ID: "shimmer", Name: "shimmer"},
		{ID: "verse", Name: "verse"},
	}
	liquidVoices = []VoiceOption{
		{ID: "American English|female", Name: "American English · female"},
		{ID: "American English|male", Name: "American English · male"},
		{ID: "British English|female", Name: "British English · female"},
		{ID: "German|female", Name: "German · female"},
	}

	catalogMu     sync.Mutex
	cachedCatalog []AudioModelChoice
	cachedAt      time.Time
)

// DiscoverCatalog gathers all available STT and TTS models across:
// 1. Remote user PCs ("Мои ПК" via KEYTRON Prime tunnel and direct LAN/Tailscale peers)
// 2. Cloud BYOK via KEYTRON Prime (/api/ai/v1/audio/models + multimodal audio models)
// 3. Direct cloud keys configured in Allan (groq, openai, openrouter, mistral, gemini, liquid)
// 4. Local engines on this PC (marked as using local CPU/GPU so 3GB GPUs aren't overloaded by default)
func DiscoverCatalog(ctx context.Context, cfg *config.Config, forceRefresh bool) []AudioModelChoice {
	catalogMu.Lock()
	if !forceRefresh && len(cachedCatalog) > 0 && time.Since(cachedAt) < 90*time.Second {
		out := append([]AudioModelChoice(nil), cachedCatalog...)
		catalogMu.Unlock()
		return out
	}
	catalogMu.Unlock()

	var (
		mu  sync.Mutex
		all []AudioModelChoice
		wg  sync.WaitGroup
	)
	appendItems := func(items ...AudioModelChoice) {
		if len(items) == 0 {
			return
		}
		mu.Lock()
		all = append(all, items...)
		mu.Unlock()
	}

	selfWorkerID := ""
	if st, _, err := link.Load(); err == nil && st != nil {
		selfWorkerID = strings.TrimSpace(st.WorkerID)
	}

	// 1. KEYTRON Prime gateway (/api/ai/v1/audio/models + multimodal realtime models)
	if kp, ok := keytronProvider(cfg); ok {
		wg.Add(1)
		go func(baseURL, apiKey string) {
			defer wg.Done()
			items := fetchKeytronAudioCatalog(ctx, baseURL, apiKey, selfWorkerID)
			appendItems(items...)
		}(kp.BaseURL, kp.APIKey)
	}

	// 2. Direct LAN / Tailscale user machines & Direct Cloud providers in cfg.Providers
	for name, p := range cfg.Providers {
		norm := backend.NormalizeProvider(name)
		if norm == "keytron" {
			continue
		}
		if isPrivateOrTailscaleURL(p.BaseURL) && !backend.IsLocalProvider(norm) {
			wg.Add(1)
			go func(provName string, prov config.ProviderConfig) {
				defer wg.Done()
				appendItems(fetchPeerDeviceAudio(ctx, provName, prov)...)
			}(name, p)
			continue
		}
		if p.APIKey != "" {
			appendItems(directCloudAudioModels(norm, p)...)
		}
	}
	// Also check active backend if not in cfg.Providers
	if cfg.Backend.APIKey != "" {
		norm := backend.NormalizeProvider(cfg.Backend.Type)
		if norm != "keytron" {
			if _, exists := cfg.Providers[norm]; !exists {
				appendItems(directCloudAudioModels(norm, config.ProviderConfig{
					Type:    cfg.Backend.Type,
					BaseURL: cfg.Backend.BaseURL,
					APIKey:  cfg.Backend.APIKey,
				})...)
			}
		}
	}

	wg.Wait()

	// 3. Local engines on this PC (placed in SecLocal so cloud BYOK / remote PC wins by default)
	if _, ok := detectWarmWhisperSocket(); ok {
		appendItems(AudioModelChoice{
			ID:       "local:faster-whisper-daemon",
			Kind:     "stt",
			Label:    "faster-whisper (daemon)",
			Group:    "Этот ПК",
			Provider: "local",
			Price:    "локально · тёплый сокет",
			Free:     true,
			Section:  SecLocal,
		})
	}
	for _, loc := range serve.ListAudio(cfg) {
		var voices []VoiceOption
		for _, v := range loc.Voices {
			voices = append(voices, VoiceOption{ID: v, Name: v})
		}
		appendItems(AudioModelChoice{
			ID:       "local:" + loc.Name,
			Kind:     loc.Kind,
			Label:    loc.Name,
			Group:    "Этот ПК",
			Provider: "local",
			Price:    "локально · CPU/VRAM",
			Free:     true,
			Section:  SecLocal,
			Voices:   voices,
		})
	}
	if bin, err := exec.LookPath("espeak-ng"); err == nil && bin != "" {
		appendItems(AudioModelChoice{
			ID:       "local:espeak-ng",
			Kind:     "tts",
			Label:    "espeak-ng",
			Group:    "Этот ПК",
			Provider: "local",
			Price:    "локально · CPU (0 VRAM)",
			Free:     true,
			Section:  SecLocal,
			Voices: []VoiceOption{
				{ID: "ru", Name: "Русский (ru)"},
				{ID: "en", Name: "English (en)"},
				{ID: "de", Name: "Deutsch (de)"},
			},
		})
	}

	// Sort deterministically by Section (SecMine -> SecBYOK -> SecDirect -> SecLocal),
	// keeping free/realtime items prominent inside their group.
	sortAudioChoices(all)

	catalogMu.Lock()
	cachedCatalog = append([]AudioModelChoice(nil), all...)
	cachedAt = time.Now()
	catalogMu.Unlock()
	return all
}

func sortAudioChoices(items []AudioModelChoice) {
	// Insertion-stable bucket sort by Section so server ordering inside each provider is preserved.
	buckets := [4][]AudioModelChoice{}
	for _, it := range items {
		sec := it.Section
		if sec < 0 || sec > 3 {
			sec = SecLocal
		}
		buckets[sec] = append(buckets[sec], it)
	}
	out := items[:0]
	for s := 0; s < 4; s++ {
		out = append(out, buckets[s]...)
	}
}

func keytronProvider(cfg *config.Config) (config.ProviderConfig, bool) {
	if cfg == nil {
		return config.ProviderConfig{}, false
	}
	if p, ok := cfg.Providers["keytron"]; ok && p.APIKey != "" {
		if p.BaseURL == "" {
			p.BaseURL = config.KeytronGateway
		}
		return p, true
	}
	if st, token, err := link.Load(); err == nil && token != "" {
		base := config.KeytronGateway
		if st != nil && st.Server != "" && !strings.Contains(st.Server, "allan.") {
			base = strings.TrimRight(st.Server, "/") + "/api/ai/v1"
		}
		return config.ProviderConfig{
			Type:    "openai",
			BaseURL: base,
			APIKey:  token,
		}, true
	}
	return config.ProviderConfig{}, false
}

func fetchKeytronAudioCatalog(parent context.Context, baseURL, apiKey, selfWorkerID string) []AudioModelChoice {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()

	base := strings.TrimRight(baseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/audio/models", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var payload struct {
		Data []struct {
			ID       string        `json:"id"`
			Kind     string        `json:"kind"` // "transcription" or "speech"
			Name     string        `json:"name"`
			Group    string        `json:"group"`
			Provider string        `json:"provider"`
			Price    string        `json:"price"`
			Free     bool          `json:"free"`
			Voices   []VoiceOption `json:"voices"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}

	var out []AudioModelChoice
	for _, m := range payload.Data {
		kind := "stt"
		if m.Kind == "speech" || m.Kind == "tts" {
			kind = "tts"
		}
		// Skip this machine's own worker entry so we don't duplicate SecLocal.
		if m.Provider == "allan" && selfWorkerID != "" && strings.HasPrefix(m.ID, "allan/"+selfWorkerID+"/") {
			continue
		}
		sec := SecBYOK
		grp := m.Group
		if grp == "" {
			grp = m.Provider
		}
		if m.Provider == "allan" {
			sec = SecMine
			grp = grp + " · через KEYTRON"
		}
		label := m.Name
		if label == "" {
			label = m.ID
		}
		isRT := strings.Contains(strings.ToLower(m.ID), "realtime") ||
			strings.Contains(strings.ToLower(m.ID), "audio") ||
			strings.Contains(strings.ToLower(m.ID), "omni") ||
			strings.Contains(strings.ToLower(m.ID), "voxtral")
		out = append(out, AudioModelChoice{
			ID:       "keytron:" + m.ID,
			Kind:     kind,
			Label:    label,
			Group:    grp,
			Provider: m.Provider,
			Price:    m.Price,
			Free:     m.Free || m.Provider == "allan" || strings.HasSuffix(m.ID, ":free"),
			Realtime: isRT,
			Section:  sec,
			Voices:   m.Voices,
		})
	}

	// Also fetch audio-capable multimodal/realtime chat models from KEYTRON /models
	// (e.g., gemini-2.5-flash, gpt-4o-audio-preview, gpt-4o-realtime-preview, voxtral)
	out = append(out, fetchKeytronMultimodalAudio(ctx, base, apiKey)...)
	return out
}

func fetchKeytronMultimodalAudio(ctx context.Context, base, apiKey string) []AudioModelChoice {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload struct {
		Data []struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			OwnedBy string   `json:"owned_by"`
			Kind    string   `json:"kind"`
			Caps    []string `json:"caps"`
			Inputs  []string `json:"inputs"`
			Free    *bool    `json:"free"`
			Worker  string   `json:"worker"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	var out []AudioModelChoice
	seen := map[string]bool{}
	for _, m := range payload.Data {
		if m.Kind != "" && m.Kind != "chat" {
			continue
		}
		hasAudio := false
		for _, c := range m.Caps {
			if c == "audio" {
				hasAudio = true
				break
			}
		}
		for _, in := range m.Inputs {
			if in == "audio" {
				hasAudio = true
				break
			}
		}
		low := strings.ToLower(m.ID)
		if !hasAudio && (strings.Contains(low, "realtime") || strings.Contains(low, "voxtral") || strings.Contains(low, "audio-preview")) {
			hasAudio = true
		}
		if !hasAudio || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		free := m.Free != nil && *m.Free
		price := "multimodal audio"
		if free {
			price = "бесплатно · multimodal"
		}
		sec := SecBYOK
		grp := m.OwnedBy + " · Realtime/Audio"
		if m.OwnedBy == "allan" && m.Worker != "" {
			sec = SecMine
			grp = m.Worker + " · Audio LLM"
		}
		label := m.Name
		if label == "" {
			label = m.ID
		}
		out = append(out, AudioModelChoice{
			ID:       "chat:keytron/" + m.ID,
			Kind:     "stt",
			Label:    label,
			Group:    grp,
			Provider: m.OwnedBy,
			Price:    price,
			Free:     free,
			Realtime: true,
			Section:  sec,
		})
	}
	return out
}

// fetchPeerDeviceAudio inspects a LAN/Tailscale endpoint of another user PC:
// - If it speaks Allan worker API (/v1/audio), imports its STT and TTS engines.
// - If it speaks Ollama (:11434), imports any ASR models (*asr*, *whisper*).
func fetchPeerDeviceAudio(parent context.Context, name string, prov config.ProviderConfig) []AudioModelChoice {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()

	u, err := url.Parse(strings.TrimRight(prov.BaseURL, "/"))
	if err != nil || u.Host == "" {
		return nil
	}
	var out []AudioModelChoice
	hostOnly := u.Hostname()

	// 1. Check if an Allan worker (/v1/audio) is reachable on the peer URL or port 8790
	workerBases := []string{strings.TrimSuffix(strings.TrimRight(prov.BaseURL, "/"), "/v1")}
	if u.Port() != "8790" && hostOnly != "" {
		workerBases = append(workerBases, fmt.Sprintf("%s://%s:8790", u.Scheme, hostOnly))
	}
	for _, wb := range workerBases {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, wb+"/v1/audio", nil)
		if err != nil {
			continue
		}
		if prov.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+prov.APIKey)
		}
		resp, err := (&http.Client{Timeout: 2500 * time.Millisecond}).Do(req)
		if err != nil {
			continue
		}
		var data struct {
			Audio []struct {
				Name   string   `json:"name"`
				Kind   string   `json:"kind"`
				Voices []string `json:"voices"`
			} `json:"audio"`
		}
		ok := resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&data) == nil
		resp.Body.Close()
		if ok && len(data.Audio) > 0 {
			for _, a := range data.Audio {
				var vs []VoiceOption
				for _, v := range a.Voices {
					vs = append(vs, VoiceOption{ID: v, Name: v})
				}
				out = append(out, AudioModelChoice{
					ID:       fmt.Sprintf("peer:%s|%s|%s", name, wb, a.Name),
					Kind:     a.Kind,
					Label:    a.Name,
					Group:    name + " · Tailscale/LAN",
					Provider: name,
					Price:    "своё железо (сеть)",
					Free:     true,
					Section:  SecMine,
					Voices:   vs,
				})
			}
			break
		}
	}

	// 2. Check if the peer runs Ollama with an ASR model (e.g. qwen3-asr)
	ollamaBase := strings.TrimSuffix(strings.TrimRight(prov.BaseURL, "/"), "/v1")
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, ollamaBase+"/api/tags", nil); err == nil {
		if resp, err := (&http.Client{Timeout: 2500 * time.Millisecond}).Do(req); err == nil {
			var tags struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}
			if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&tags) == nil {
				for _, m := range tags.Models {
					low := strings.ToLower(m.Name)
					if strings.Contains(low, "asr") || strings.Contains(low, "whisper") || strings.Contains(low, "audio") {
						out = append(out, AudioModelChoice{
							ID:       fmt.Sprintf("peer-ollama:%s|%s|%s", name, ollamaBase, m.Name),
							Kind:     "stt",
							Label:    m.Name,
							Group:    name + " · Ollama LAN",
							Provider: name,
							Price:    "своё железо (сеть)",
							Free:     true,
							Section:  SecMine,
						})
					}
				}
			}
			resp.Body.Close()
		}
	}
	return out
}

func directCloudAudioModels(provider string, p config.ProviderConfig) []AudioModelChoice {
	switch provider {
	case "groq":
		return []AudioModelChoice{
			{ID: "direct:groq/whisper-large-v3-turbo", Kind: "stt", Label: "Whisper Large v3 Turbo", Group: "Groq", Provider: "groq", Price: "$0.0007/мин", Section: SecDirect},
			{ID: "direct:groq/whisper-large-v3", Kind: "stt", Label: "Whisper Large v3", Group: "Groq", Provider: "groq", Price: "$0.0018/мин", Section: SecDirect},
		}
	case "openai":
		return []AudioModelChoice{
			{ID: "direct:openai/gpt-4o-mini-transcribe", Kind: "stt", Label: "GPT-4o mini Transcribe", Group: "OpenAI", Provider: "openai", Price: "$0.003/мин", Section: SecDirect},
			{ID: "direct:openai/gpt-4o-transcribe", Kind: "stt", Label: "GPT-4o Transcribe", Group: "OpenAI", Provider: "openai", Price: "$0.006/мин", Section: SecDirect},
			{ID: "direct:openai/whisper-1", Kind: "stt", Label: "Whisper v2", Group: "OpenAI", Provider: "openai", Price: "$0.006/мин", Section: SecDirect},
			{ID: "chat:openai/gpt-4o-mini-audio-preview", Kind: "stt", Label: "GPT-4o mini Audio (Realtime/Audio)", Group: "OpenAI", Provider: "openai", Price: "multimodal", Realtime: true, Section: SecDirect},
			{ID: "direct:openai/gpt-4o-mini-tts", Kind: "tts", Label: "GPT-4o mini TTS", Group: "OpenAI", Provider: "openai", Price: "$12/1M", Section: SecDirect, Voices: openAIVoices},
			{ID: "direct:openai/tts-1", Kind: "tts", Label: "TTS-1", Group: "OpenAI", Provider: "openai", Price: "$15/1M", Section: SecDirect, Voices: openAIVoices},
			{ID: "direct:openai/tts-1-hd", Kind: "tts", Label: "TTS-1 HD", Group: "OpenAI", Provider: "openai", Price: "$30/1M", Section: SecDirect, Voices: openAIVoices},
		}
	case "mistral":
		return []AudioModelChoice{
			{ID: "direct:mistral/voxtral-mini-latest", Kind: "stt", Label: "Voxtral Mini Transcribe", Group: "Mistral", Provider: "mistral", Price: "$0.001/мин", Realtime: true, Section: SecDirect},
			{ID: "chat:mistral/voxtral-small-latest", Kind: "stt", Label: "Voxtral Small Multimodal", Group: "Mistral", Provider: "mistral", Price: "multimodal", Realtime: true, Section: SecDirect},
		}
	case "gemini":
		return []AudioModelChoice{
			{ID: "chat:gemini/gemini-2.5-flash", Kind: "stt", Label: "Gemini 2.5 Flash Audio", Group: "Gemini", Provider: "gemini", Price: "multimodal", Realtime: true, Section: SecDirect},
		}
	case "liquid":
		return []AudioModelChoice{
			{ID: "liquid:lfm3-audio-2b", Kind: "stt", Label: "LFM3 Audio 2B", Group: "Liquid AI", Provider: "liquid", Price: "LFM3 Audio", Realtime: true, Section: SecDirect},
			{ID: "liquid:lfm3-omni-2b", Kind: "stt", Label: "LFM3 Omni 2B", Group: "Liquid AI", Provider: "liquid", Price: "LFM3 Omni", Realtime: true, Section: SecDirect},
			{ID: "liquid:lfm3-audio-2b", Kind: "tts", Label: "LFM3 Audio 2B TTS", Group: "Liquid AI", Provider: "liquid", Price: "LFM3 Audio", Realtime: true, Section: SecDirect, Voices: liquidVoices},
		}
	case "openrouter":
		return []AudioModelChoice{
			{ID: "chat:openrouter/google/gemini-2.5-flash", Kind: "stt", Label: "Gemini 2.5 Flash (Audio)", Group: "OpenRouter", Provider: "openrouter", Price: "копейки · audio", Realtime: true, Section: SecDirect},
			{ID: "chat:openrouter/openai/gpt-4o-mini-audio-preview", Kind: "stt", Label: "GPT-4o mini Audio", Group: "OpenRouter", Provider: "openrouter", Price: "audio", Realtime: true, Section: SecDirect},
			{ID: "openrouter-tts:openai/gpt-4o-mini-audio-preview", Kind: "tts", Label: "GPT-4o mini Audio TTS", Group: "OpenRouter", Provider: "openrouter", Price: "audio", Realtime: true, Section: SecDirect, Voices: openAIVoices},
		}
	}
	return nil
}

// ResolveDefaultModel picks the best available STT or TTS model when none is
// pinned in config, preferring user's remote PCs ("Мои ПК") and Cloud BYOK
// over local VRAM-heavy models!
func ResolveDefaultModel(ctx context.Context, cfg *config.Config, kind string) (AudioModelChoice, bool) {
	items := DiscoverCatalog(ctx, cfg, false)
	if good := AutoModel(kind); good != "" {
		for _, it := range items {
			if it.ID == good {
				return it, true
			}
		}
	}
	if c := autoCandidates(ctx, cfg, kind); len(c) > 0 {
		return c[0], true
	}
	var firstLocal *AudioModelChoice
	// Prefer non-chat dedicated transcription/speech endpoints first in SecMine / SecBYOK / SecDirect
	for i := range items {
		it := &items[i]
		if it.Kind != kind {
			continue
		}
		if it.Section == SecLocal {
			if firstLocal == nil {
				firstLocal = it
			}
			continue
		}
		if !strings.HasPrefix(it.ID, "chat:") {
			return *it, true
		}
	}
	// Next, any multimodal cloud model (SecMine / SecBYOK / SecDirect)
	for i := range items {
		it := &items[i]
		if it.Kind == kind && it.Section != SecLocal {
			return *it, true
		}
	}
	if firstLocal != nil {
		return *firstLocal, true
	}
	return AudioModelChoice{}, false
}

// ShortModelLabel formats a canonical voice model ID nicely for the TUI dock.
func ShortModelLabel(id string) string {
	for _, p := range []string{"keytron:", "direct:", "local:", "chat:keytron/", "chat:", "liquid:", "openrouter-tts:"} {
		if strings.HasPrefix(id, p) {
			s := strings.TrimPrefix(id, p)
			if strings.HasPrefix(p, "keytron") {
				return "☁ " + s
			}
			if p == "local:" {
				return "💻 " + s
			}
			return "☁ " + s
		}
	}
	if strings.HasPrefix(id, "peer:") || strings.HasPrefix(id, "peer-ollama:") {
		parts := strings.Split(id, "|")
		if len(parts) == 3 {
			host := strings.TrimPrefix(strings.TrimPrefix(parts[0], "peer:"), "peer-ollama:")
			return "🖥 " + host + "/" + parts[2]
		}
	}
	return id
}

// executeCloudOrRemoteSTT handles any non-local STT model ID.
func executeCloudOrRemoteSTT(ctx context.Context, cfg *config.Config, spec, lang string, wav []byte) (string, string, error) {
	switch {
	case strings.HasPrefix(spec, "keytron:"):
		modelID := strings.TrimPrefix(spec, "keytron:")
		kp, ok := keytronProvider(cfg)
		if !ok {
			return "", spec, fmt.Errorf("KEYTRON Prime не подключён (/connect или allan key set keytron)")
		}
		txt, err := multipartTranscribe(ctx, strings.TrimRight(kp.BaseURL, "/")+"/audio/transcriptions", kp.APIKey, modelID, lang, wav)
		return txt, ShortModelLabel(spec), err

	case strings.HasPrefix(spec, "direct:"):
		rest := strings.TrimPrefix(spec, "direct:")
		prov, modelID, _ := strings.Cut(rest, "/")
		baseURL, apiKey, err := providerCreds(cfg, prov)
		if err != nil {
			return "", spec, err
		}
		txt, err := multipartTranscribe(ctx, strings.TrimRight(baseURL, "/")+"/audio/transcriptions", apiKey, modelID, lang, wav)
		return txt, ShortModelLabel(spec), err

	case strings.HasPrefix(spec, "chat:"):
		rest := strings.TrimPrefix(spec, "chat:")
		prov, modelID, _ := strings.Cut(rest, "/")
		baseURL, apiKey, err := providerCreds(cfg, prov)
		if err != nil {
			return "", spec, err
		}
		txt, err := chatAudioTranscribe(ctx, baseURL, apiKey, prov, modelID, lang, wav)
		return txt, ShortModelLabel(spec), err

	case strings.HasPrefix(spec, "liquid:"):
		modelID := strings.TrimPrefix(spec, "liquid:")
		baseURL, apiKey, err := providerCreds(cfg, "liquid")
		if err != nil {
			return "", spec, err
		}
		txt, err := chatAudioTranscribe(ctx, baseURL, apiKey, "liquid", modelID, lang, wav)
		return txt, "☁ liquid/" + modelID, err

	case strings.HasPrefix(spec, "peer:"):
		parts := strings.SplitN(strings.TrimPrefix(spec, "peer:"), "|", 3)
		if len(parts) != 3 {
			return "", spec, fmt.Errorf("некорректный адрес воркера: %s", spec)
		}
		provName, wb, eng := parts[0], parts[1], parts[2]
		apiKey := cfg.Providers[provName].APIKey
		txt, err := peerWorkerTranscribe(ctx, wb, apiKey, eng, lang, wav)
		return txt, fmt.Sprintf("🖥 %s/%s", provName, eng), err

	case strings.HasPrefix(spec, "peer-ollama:"):
		parts := strings.SplitN(strings.TrimPrefix(spec, "peer-ollama:"), "|", 3)
		if len(parts) != 3 {
			return "", spec, fmt.Errorf("некорректный адрес Ollama: %s", spec)
		}
		provName, ollamaBase, modelName := parts[0], parts[1], parts[2]
		txt, err := remoteOllamaASR(ctx, ollamaBase, modelName, lang, wav)
		return txt, fmt.Sprintf("🖥 %s/%s", provName, modelName), err
	}

	// Bare "provider/model": if keytron is configured and knows it, route through keytron;
	// otherwise try direct provider credentials.
	if strings.Contains(spec, "/") && !strings.HasPrefix(spec, "local:") {
		prov, modelID, _ := strings.Cut(spec, "/")
		if pBase, pKey, err := providerCreds(cfg, prov); err == nil && pKey != "" {
			txt, err := multipartTranscribe(ctx, strings.TrimRight(pBase, "/")+"/audio/transcriptions", pKey, modelID, lang, wav)
			return txt, "☁ " + spec, err
		}
		if kp, ok := keytronProvider(cfg); ok {
			txt, err := multipartTranscribe(ctx, strings.TrimRight(kp.BaseURL, "/")+"/audio/transcriptions", kp.APIKey, spec, lang, wav)
			return txt, "☁ " + spec, err
		}
	}
	return "", "", fmt.Errorf("неизвестная модель распознавания: %s", spec)
}

// executeCloudOrRemoteTTS handles any non-local TTS model ID and normalizes the
// returned audio (MP3/Opus/WAV) into 16 kHz mono WAV so the visualizer animates.
func executeCloudOrRemoteTTS(ctx context.Context, cfg *config.Config, spec, voiceID, lang, text string) ([]byte, string, error) {
	var (
		rawAudio []byte
		label    = ShortModelLabel(spec)
		err      error
	)
	switch {
	case strings.HasPrefix(spec, "keytron:"):
		modelID := strings.TrimPrefix(spec, "keytron:")
		kp, ok := keytronProvider(cfg)
		if !ok {
			return nil, spec, fmt.Errorf("KEYTRON Prime не подключён (/connect или allan key set keytron)")
		}
		rawAudio, err = jsonSpeechRequest(ctx, strings.TrimRight(kp.BaseURL, "/")+"/audio/speech", kp.APIKey, map[string]any{
			"model":    modelID,
			"input":    text,
			"voice":    voiceID,
			"language": lang,
		})

	case strings.HasPrefix(spec, "direct:"):
		rest := strings.TrimPrefix(spec, "direct:")
		prov, modelID, _ := strings.Cut(rest, "/")
		baseURL, apiKey, cErr := providerCreds(cfg, prov)
		if cErr != nil {
			return nil, spec, cErr
		}
		v := voiceID
		if v == "" {
			v = "alloy"
		}
		rawAudio, err = jsonSpeechRequest(ctx, strings.TrimRight(baseURL, "/")+"/audio/speech", apiKey, map[string]any{
			"model":           modelID,
			"input":           text,
			"voice":           v,
			"response_format": "wav",
		})

	case strings.HasPrefix(spec, "openrouter-tts:"), strings.HasPrefix(spec, "liquid:"):
		isLiquid := strings.HasPrefix(spec, "liquid:")
		prov := "openrouter"
		modelID := strings.TrimPrefix(spec, "openrouter-tts:")
		if isLiquid {
			prov = "liquid"
			modelID = strings.TrimPrefix(spec, "liquid:")
		}
		baseURL, apiKey, cErr := providerCreds(cfg, prov)
		if cErr != nil {
			return nil, spec, cErr
		}
		rawAudio, err = chatAudioSpeak(ctx, baseURL, apiKey, prov, modelID, voiceID, text)

	case strings.HasPrefix(spec, "peer:"):
		parts := strings.SplitN(strings.TrimPrefix(spec, "peer:"), "|", 3)
		if len(parts) != 3 {
			return nil, spec, fmt.Errorf("некорректный адрес воркера: %s", spec)
		}
		provName, wb, eng := parts[0], parts[1], parts[2]
		apiKey := cfg.Providers[provName].APIKey
		rawAudio, err = jsonSpeechRequest(ctx, wb+"/v1/audio/speech", apiKey, map[string]any{
			"model":    eng,
			"input":    text,
			"voice":    voiceID,
			"language": lang,
		})
		label = fmt.Sprintf("🖥 %s/%s", provName, eng)

	default:
		if strings.Contains(spec, "/") && !strings.HasPrefix(spec, "local:") {
			if kp, ok := keytronProvider(cfg); ok {
				rawAudio, err = jsonSpeechRequest(ctx, strings.TrimRight(kp.BaseURL, "/")+"/audio/speech", kp.APIKey, map[string]any{
					"model":    spec,
					"input":    text,
					"voice":    voiceID,
					"language": lang,
				})
				label = "☁ " + spec
			}
		} else {
			return nil, spec, fmt.Errorf("неизвестная модель озвучки: %s", spec)
		}
	}

	if err != nil {
		return nil, label, err
	}
	wav16k, convErr := NormalizeAudioToWAV16k(ctx, rawAudio)
	if convErr != nil {
		return rawAudio, label, nil
	}
	return wav16k, label, nil
}

func providerCreds(cfg *config.Config, provider string) (baseURL, apiKey string, err error) {
	norm := backend.NormalizeProvider(provider)
	if norm == "keytron" {
		if kp, ok := keytronProvider(cfg); ok {
			return kp.BaseURL, kp.APIKey, nil
		}
		return "", "", fmt.Errorf("провайдер keytron не подключён (/connect)")
	}
	if p, ok := cfg.Providers[norm]; ok && p.APIKey != "" {
		base := p.BaseURL
		if base == "" {
			base = backend.DefaultBaseURL(norm)
		}
		return base, p.APIKey, nil
	}
	if backend.NormalizeProvider(cfg.Backend.Type) == norm && cfg.Backend.APIKey != "" {
		base := cfg.Backend.BaseURL
		if base == "" {
			base = backend.DefaultBaseURL(norm)
		}
		return base, cfg.Backend.APIKey, nil
	}
	return "", "", fmt.Errorf("нет API-ключа для %s (/api add %s <ключ>)", norm, norm)
}

func multipartTranscribe(ctx context.Context, endpoint, apiKey, modelID, lang string, wav []byte) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "speech.wav")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(wav); err != nil {
		return "", err
	}
	_ = mw.WriteField("model", modelID)
	_ = mw.WriteField("response_format", "json")
	if lang != "" && lang != "auto" {
		_ = mw.WriteField("language", lang)
	}
	_ = mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, extractAPIErr(raw))
	}
	var res struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return strings.TrimSpace(string(raw)), nil
	}
	return strings.TrimSpace(res.Text), nil
}

// chatAudioTranscribe sends WAV audio via OpenAI-compatible multimodal `input_audio`
// content block (supported by KEYTRON Prime, OpenRouter, Gemini, OpenAI Audio, Voxtral, Liquid LFM3-Audio).
func chatAudioTranscribe(ctx context.Context, baseURL, apiKey, provider, modelID, lang string, wav []byte) (string, error) {
	sysTask := "Transcribe the user's speech accurately. Reply with ONLY the transcribed text, without quotes or commentary."
	if provider == "liquid" {
		langFull := map[string]string{"ru": "Russian", "en": "English", "de": "German"}[lang]
		if langFull != "" {
			sysTask = "Perform ASR in " + langFull + "."
		} else {
			sysTask = "Perform ASR."
		}
	}
	payload := map[string]any{
		"model":       modelID,
		"temperature": 0,
		"messages": []map[string]any{
			{"role": "system", "content": sysTask},
			{"role": "user", "content": []map[string]any{
				{
					"type": "input_audio",
					"input_audio": map[string]string{
						"data":   base64.StdEncoding.EncodeToString(wav),
						"format": "wav",
					},
				},
			}},
		},
	}
	rawBody, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if provider == "openrouter" {
		req.Header.Set("HTTP-Referer", "https://keytron-prime.org")
		req.Header.Set("X-Title", "Allan CLI")
	}

	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, extractAPIErr(raw))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

func chatAudioSpeak(ctx context.Context, baseURL, apiKey, provider, modelID, voiceID, text string) ([]byte, error) {
	sys := "Read the user's message aloud naturally, verbatim."
	v := voiceID
	if provider == "liquid" {
		langPart, genderPart, ok := strings.Cut(voiceID, "|")
		if !ok || langPart == "" {
			langPart, genderPart = "American English", "female"
		}
		if genderPart == "" {
			genderPart = "female"
		}
		sys = fmt.Sprintf("Perform TTS in %s using a %s voice.", langPart, genderPart)
		v = "alloy"
	} else if v == "" {
		v = "alloy"
	}
	payload := map[string]any{
		"model":      modelID,
		"modalities": []string{"text", "audio"},
		"audio":      map[string]string{"voice": v, "format": "wav"},
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": text},
		},
	}
	rawBody, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 24<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, extractAPIErr(raw))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Audio struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Audio.Data == "" {
		return nil, fmt.Errorf("модель не вернула аудиопоток")
	}
	return base64.StdEncoding.DecodeString(parsed.Choices[0].Message.Audio.Data)
}

func jsonSpeechRequest(ctx context.Context, endpoint, apiKey string, payload map[string]any) ([]byte, error) {
	rawBody, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, extractAPIErr(raw))
	}
	return raw, nil
}

func peerWorkerTranscribe(ctx context.Context, wb, apiKey, engine, lang string, wav []byte) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"model":    engine,
		"audio":    base64.StdEncoding.EncodeToString(wav),
		"format":   "wav",
		"language": lang,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(wb, "/")+"/v1/audio/transcriptions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, extractAPIErr(raw))
	}
	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	return strings.TrimSpace(parsed.Text), nil
}

func remoteOllamaASR(ctx context.Context, ollamaBase, modelName, lang string, wav []byte) (string, error) {
	// Как ollamaASR в allan serve: Qwen3-ASR в Ollama берёт WAV полем "images",
	// а заранее начатый ответ "language X<asr_text>" закрепляет язык — без него
	// смешанная речь может вернуться переводом. Ответ: "language X<asr_text>текст".
	messages := []map[string]any{{"role": "user", "content": "", "images": []string{base64.StdEncoding.EncodeToString(wav)}}}
	if n := map[string]string{"ru": "Russian", "en": "English", "de": "German"}[strings.ToLower(lang)]; n != "" {
		messages = append(messages, map[string]any{"role": "assistant", "content": "language " + n + "<asr_text>"})
	}
	payload, _ := json.Marshal(map[string]any{
		"model": modelName, "stream": false, "messages": messages,
		"options": map[string]any{"num_ctx": 2048, "temperature": 0},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(ollamaBase, "/")+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, extractAPIErr(raw))
	}
	var parsed struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("Ollama: %s", parsed.Error)
	}
	text := parsed.Message.Content
	if i := strings.Index(text, "<asr_text>"); i >= 0 {
		text = text[i+len("<asr_text>"):]
	}
	return strings.TrimSpace(text), nil
}

// NormalizeAudioToWAV16k converts compressed cloud audio (MP3, Opus/OGG, AAC, FLAC, or WAV
// of any sample rate) into 16 kHz mono s16le WAV so both playback and the 16-band
// TUI visualizer work seamlessly.
func NormalizeAudioToWAV16k(ctx context.Context, audioBytes []byte) ([]byte, error) {
	if len(audioBytes) >= 44 && string(audioBytes[0:4]) == "RIFF" && string(audioBytes[8:12]) == "WAVE" {
		if pcm, err := DecodeWAVTo16kMono(audioBytes); err == nil && len(pcm) > 0 {
			return EncodeWAV16kMono(pcm, SampleRate), nil
		}
		return audioBytes, nil
	}
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, err
	}
	inTmp, err := os.CreateTemp("", "allan-cloud-audio-*")
	if err != nil {
		return nil, err
	}
	inPath := inTmp.Name()
	_, _ = inTmp.Write(audioBytes)
	_ = inTmp.Close()
	defer os.Remove(inPath)

	outPath := inPath + ".wav"
	defer os.Remove(outPath)

	cmd := exec.CommandContext(ctx, bin, "-y", "-loglevel", "error", "-i", inPath, "-ac", "1", "-ar", "16000", "-sample_fmt", "s16", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg decode: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(outPath)
}

func extractAPIErr(raw []byte) string {
	var parsed struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Error != nil {
		switch e := parsed.Error.(type) {
		case string:
			return e
		case map[string]any:
			if msg, ok := e["message"].(string); ok && msg != "" {
				return msg
			}
		}
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func isPrivateOrTailscaleURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if h == "" || h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return false
	}
	if strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".lan") || strings.HasSuffix(h, ".ts.net") {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return !strings.Contains(h, ".")
	}
	_, tailscaleCGNAT, _ := net.ParseCIDR("100.64.0.0/10")
	return ip.IsPrivate() || (tailscaleCGNAT != nil && tailscaleCGNAT.Contains(ip))
}

func detectWarmWhisperSocket() (string, bool) {
	stateDir := os.Getenv("XDG_RUNTIME_DIR")
	if stateDir == "" {
		stateDir = os.TempDir()
	}
	sockPath := stateDir + "/claude-voice/stt.sock"
	if _, err := os.Stat(sockPath); err == nil {
		return sockPath, true
	}
	return "", false
}
