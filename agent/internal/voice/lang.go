package voice

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SpeechLang — язык, на котором человек говорит. Он не обязан совпадать с языком
// интерфейса: интерфейс можно держать английским и говорить по-русски. А модели
// с подсказкой языка (Qwen3-ASR, whisper) при неверной подсказке не распознают,
// а переводят речь. Поэтому: явная настройка voice_lang; "auto" — без подсказки;
// пусто — язык из локали системы (LC_ALL, LC_MESSAGES, LANG).
func SpeechLang(setting string) string {
	v := strings.ToLower(strings.TrimSpace(setting))
	if v == "auto" {
		return ""
	}
	if v != "" {
		return v
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		loc := strings.ToLower(os.Getenv(key))
		if loc == "" || loc == "c" || loc == "posix" || strings.HasPrefix(loc, "c.") {
			continue
		}
		if i := strings.IndexAny(loc, "_.@"); i > 0 {
			loc = loc[:i]
		}
		if len(loc) == 2 || len(loc) == 3 {
			return loc
		}
	}
	return ""
}

// logAttempt дописывает строку в ~/.allan/voice.log: какая модель, сколько
// заняло и чем кончилось. Текст речи не пишем — только его длину.
func logAttempt(kind, model, lang string, took time.Duration, result string, err error) {
	home, hErr := os.UserHomeDir()
	if hErr != nil {
		return
	}
	f, oErr := os.OpenFile(filepath.Join(home, ".allan", "voice.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if oErr != nil {
		return
	}
	defer f.Close()
	status := result
	if err != nil {
		status = "ошибка: " + strings.ReplaceAll(err.Error(), "\n", " ")
		if len(status) > 300 {
			status = status[:300] + "…"
		}
	}
	if lang == "" {
		lang = "auto"
	}
	fmt.Fprintf(f, "%s %s lang=%s %s %dms %s\n", time.Now().Format("2006-01-02 15:04:05"), kind, lang, model, took.Milliseconds(), status)
}
