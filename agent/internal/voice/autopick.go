package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/keytron/allan/agent/config"
)

// Авторежим (модель не закреплена в конфиге): облачных и чужих моделей много,
// но у части провайдеров в любой момент может кончиться баланс или сработать
// лимит. Поэтому пробуем несколько кандидатов по очереди, запоминаем первую
// сработавшую модель на всю сессию, а упавшую на время убираем из очереди.

var errEmptyTranscript = errors.New("модели вернули пустой текст")

const (
	maxAutoAttempts = 5
	failCooldown    = 10 * time.Minute
)

var (
	autoMu      sync.Mutex
	autoGood    = map[string]string{}    // kind → id последней сработавшей модели
	autoFailed  = map[string]time.Time{} // id → до какого момента не пробовать
	autoLastErr = map[string]string{}    // id → текст последней ошибки (для /voice)
)

func markAutoGood(kind, id string) {
	autoMu.Lock()
	autoGood[kind] = id
	delete(autoFailed, id)
	delete(autoLastErr, id)
	autoMu.Unlock()
}

func markAutoFailed(kind, id string, err error) {
	autoMu.Lock()
	autoFailed[id] = time.Now().Add(failCooldown)
	if err != nil {
		autoLastErr[id] = err.Error()
	}
	if autoGood[kind] == id {
		delete(autoGood, kind)
	}
	autoMu.Unlock()
}

// AutoModel возвращает модель, которая сработала последней в авторежиме, если такая есть.
func AutoModel(kind string) string {
	autoMu.Lock()
	defer autoMu.Unlock()
	return autoGood[kind]
}

// autoCandidates — облачные/удалённые модели нужного вида в порядке попыток.
func autoCandidates(ctx context.Context, cfg *config.Config, kind string) []AudioModelChoice {
	return AutoOrder(DiscoverCatalog(ctx, cfg, false), kind)
}

// AutoOrder упорядочивает уже загруженный каталог так, как его перебирает авторежим:
// последняя сработавшая модель, затем свои ПК, бесплатные, остальные BYOK, прямые
// ключи; мультимодальные chat-модели — после выделенных STT/TTS-эндпоинтов.
// Модели, недавно упавшие, пропускаются. В сеть не ходит — годится для отрисовки.
func AutoOrder(items []AudioModelChoice, kind string) []AudioModelChoice {
	now := time.Now()
	autoMu.Lock()
	good := autoGood[kind]
	failed := make(map[string]time.Time, len(autoFailed))
	for k, v := range autoFailed {
		failed[k] = v
	}
	autoMu.Unlock()

	rank := func(it AudioModelChoice) int {
		r := it.Section * 10
		if it.Section == SecBYOK && !it.Free {
			r += 5
		}
		if strings.HasPrefix(it.ID, "chat:") {
			r += 100
		}
		return r
	}
	var first, rest []AudioModelChoice
	for _, it := range items {
		if it.Kind != kind || it.Section == SecLocal || strings.HasPrefix(it.ID, "local:") {
			continue
		}
		if until, ok := failed[it.ID]; ok && now.Before(until) {
			continue
		}
		if it.ID == good {
			first = append(first, it)
			continue
		}
		rest = append(rest, it)
	}
	// стабильная сортировка вставками по рангу: порядок сервера внутри ранга сохраняется
	for i := 1; i < len(rest); i++ {
		for j := i; j > 0 && rank(rest[j]) < rank(rest[j-1]); j-- {
			rest[j], rest[j-1] = rest[j-1], rest[j]
		}
	}
	return append(first, rest...)
}

// autoTranscribe перебирает кандидатов; ok=false — ни один не сработал.
func autoTranscribe(ctx context.Context, cfg *config.Config, wav []byte, lang string) (string, string, error, bool) {
	var lastErr error
	tries := 0
	for _, c := range autoCandidates(ctx, cfg, "stt") {
		if tries >= maxAutoAttempts || ctx.Err() != nil {
			break
		}
		tries++
		t0 := time.Now()
		txt, label, err := executeCloudOrRemoteSTT(ctx, cfg, c.ID, lang, wav)
		if err == nil && strings.TrimSpace(txt) == "" {
			// речь была (тишину сюда не шлём), а текста нет — модель не справилась
			// (например, ещё грузится на холодном старте): пробуем следующую,
			// но надолго не выкидываем
			logAttempt("stt", c.ID, lang, time.Since(t0), "пусто → следующая", nil)
			lastErr = errEmptyTranscript
			continue
		}
		logAttempt("stt", c.ID, lang, time.Since(t0), fmt.Sprintf("ok, %d симв.", len([]rune(txt))), err)
		if err == nil {
			markAutoGood("stt", c.ID)
			return txt, label, nil, true
		}
		markAutoFailed("stt", c.ID, err)
		lastErr = err
	}
	return "", "", lastErr, false
}

// autoSynthesize перебирает кандидатов; ok=false — ни один не сработал.
func autoSynthesize(ctx context.Context, cfg *config.Config, text, lang, voiceID string) ([]byte, string, error, bool) {
	var lastErr error
	tries := 0
	for _, c := range autoCandidates(ctx, cfg, "tts") {
		if tries >= maxAutoAttempts || ctx.Err() != nil {
			break
		}
		tries++
		v := voiceID
		if v == "" && len(c.Voices) > 0 {
			v = pickBestVoiceOption(c.Voices, lang, text)
		}
		t0 := time.Now()
		w, label, err := executeCloudOrRemoteTTS(ctx, cfg, c.ID, v, lang, text)
		logAttempt("tts", c.ID, lang, time.Since(t0), fmt.Sprintf("ok, %d байт", len(w)), err)
		if err == nil && len(w) > 0 {
			markAutoGood("tts", c.ID)
			return w, label, nil, true
		}
		markAutoFailed("tts", c.ID, err)
		lastErr = err
	}
	return nil, "", lastErr, false
}
