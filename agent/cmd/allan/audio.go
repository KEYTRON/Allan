package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/audiosetup"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/serve"
)

const audioUsage = `allan audio — голос на этой машине для чата KEYTRON Prime

  allan audio              что машина умеет: распознавание и озвучка
  allan audio setup        поставить движки (без root): диктовка macOS,
                           faster-whisper, голоса Piper
  allan audio setup --yes  то же без вопросов

Сайт видит эти движки, пока запущен allan serve.
`

func runAudio(args []string) int {
	if len(args) == 0 {
		return audioStatus()
	}
	switch args[0] {
	case "setup":
		yes := len(args) > 1 && (args[1] == "--yes" || args[1] == "-y")
		return audioSetup(yes)
	case "help", "--help", "-h":
		fmt.Print(i18n.S(audioUsage))
		return 0
	}
	fmt.Fprint(os.Stderr, i18n.S(audioUsage))
	return 2
}

func audioStatus() int {
	cfg, _, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	engines := serve.ListAudio(cfg)
	if len(engines) == 0 {
		fmt.Println(i18n.S("Движков речи на этой машине нет. Поставить: allan audio setup"))
		return 0
	}
	for _, kind := range []string{"stt", "tts"} {
		title := i18n.S("Распознавание речи")
		if kind == "tts" {
			title = i18n.S("Озвучка")
		}
		fmt.Println(title)
		for _, e := range engines {
			if e.Kind != kind {
				continue
			}
			line := "  " + e.Name
			if len(e.Voices) > 0 {
				shown := e.Voices
				if len(shown) > 6 {
					shown = shown[:6]
				}
				line += " · " + strings.Join(shown, ", ")
				if len(e.Voices) > 6 {
					line += fmt.Sprintf(" … +%d", len(e.Voices)-6)
				}
			}
			fmt.Println(line)
		}
	}
	return 0
}

// one reader for all questions: a reader per question would swallow the next answers from a pipe
var stdinReader = bufio.NewReader(os.Stdin)

func ask(yes bool, question string) bool {
	if yes {
		return true
	}
	fmt.Print(question + " [y/N] ")
	answer, _ := stdinReader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes" || answer == "\u0434" || answer == "\u0434\u0430" // «д», «да»
}

func audioSetup(yes bool) int {
	failed := false
	if runtime.GOOS == "darwin" {
		switch {
		case serve.AppleDictationApp() != "":
			fmt.Println(i18n.S("✓ диктовка macOS уже настроена"))
		case !audiosetup.CanBuildMacHelper():
			fmt.Println(i18n.S("· диктовка macOS: нужен компилятор Swift — xcode-select --install"))
		case ask(yes, i18n.S("Настроить диктовку macOS (распознавание на этом Mac, бесплатно)? macOS один раз спросит разрешение.")):
			if granted, err := audiosetup.BuildMacHelper(os.Stdout); err != nil {
				fmt.Fprint(os.Stderr, i18n.F("✗ диктовка macOS: %v\n", err))
				failed = true
			} else if granted {
				fmt.Println(i18n.S("✓ диктовка macOS готова"))
			}
		}
		fmt.Println(i18n.S("· озвучка macOS (say) работает без установки; русский голос Milena докачивается в Системных настройках → Универсальный доступ → Устный контент → Системный голос"))
	}
	if ask(yes, i18n.S("Поставить faster-whisper (распознавание, быстрее openai-whisper; модель скачается при первом запросе)?")) {
		if err := audiosetup.InstallFasterWhisper(os.Stdout); err != nil {
			fmt.Fprint(os.Stderr, i18n.F("✗ faster-whisper: %v\n", err))
			failed = true
		} else {
			fmt.Println(i18n.S("✓ faster-whisper готов"))
		}
	}
	if ask(yes, i18n.S("Поставить Piper (озвучка) с русским и английским голосом, ~130 МБ?")) {
		if err := audiosetup.InstallPiper(os.Stdout); err != nil {
			fmt.Fprint(os.Stderr, i18n.F("✗ Piper: %v\n", err))
			failed = true
		} else {
			fmt.Println(i18n.S("✓ Piper готов"))
		}
	}
	fmt.Println()
	audioStatus()
	fmt.Println(i18n.S("\nЧтобы сайт увидел новые движки, перезапустите воркер: allan serve stop && allan serve"))
	if failed {
		return 1
	}
	return 0
}
