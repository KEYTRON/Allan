package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/agent"
	"github.com/keytron/allan/agent/internal/bootstrap"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/link"
	"github.com/keytron/allan/agent/internal/tui"
)

// scanLangFlag finds --lang/--lang=… before the flag package runs, so the help
// text is already in the right language.
func scanLangFlag(args []string) string {
	for i, a := range args {
		switch {
		case a == "--lang" && i+1 < len(args):
			return args[i+1]
		case strings.HasPrefix(a, "--lang="):
			return strings.TrimPrefix(a, "--lang=")
		}
	}
	return ""
}

var version = "0.4.0"

// resumeFlag accepts both "allan --resume" and "allan --resume <id>" (also
// "allan --resume=<id>"). Go's flag package cannot do that with a plain Bool:
// a string flag would demand a value, and a bool flag would reject one.
type resumeFlag struct {
	set bool
	id  string
}

func (r *resumeFlag) String() string {
	if r.id == "" {
		return ""
	}
	return r.id
}

func (r *resumeFlag) Set(v string) error {
	r.set = true
	if v != "true" && v != "" {
		r.id = v
	}
	return nil
}

// IsBoolFlag makes "--resume" valid without a value.
func (r *resumeFlag) IsBoolFlag() bool { return true }

func main() {
	// The language must be known before the help text and the flag descriptions
	// are built, so --lang is picked out of the arguments up front.
	if langFlag := scanLangFlag(os.Args[1:]); langFlag != "" {
		i18n.Set(i18n.Normalize(langFlag))
	} else {
		if l := i18n.Normalize(config.LangFromFile()); l != "" {
			i18n.Set(l)
		} else {
			i18n.Set(i18n.Detect())
		}
	}

	var (
		flBackend   = flag.String("backend", "", i18n.S("провайдер"))
		flModel     = flag.String("model", "", i18n.S("модель"))
		flWorkspace = flag.String("workspace", "", i18n.S("рабочая папка"))
		flVersion   = flag.Bool("version", false, i18n.S("версия"))
		flNoMemory  = flag.Bool("no-memory", false, i18n.S("без памяти"))
		flLang      = flag.String("lang", "", i18n.S("язык интерфейса: ru, en, de"))
		flResume    = &resumeFlag{}
	)
	// The value of --lang was already read by scanLangFlag; the flag itself only
	// has to exist so the parser accepts it.
	_ = flLang
	flag.Var(flResume, "resume", i18n.S("продолжить сессию: --resume [id] (без id — последняя)"))
	flag.Usage = printUsage
	flag.Parse()

	// Everything from the first non-flag word onwards belongs to a subcommand:
	// both `allan key set openrouter` and `allan --lang en serve` must work.
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "key":
			os.Exit(runKey(args[1:]))
		case "serve":
			os.Exit(runServe(args[1:]))
		case "connect":
			os.Exit(runConnect(args[1:]))
		case "id":
			os.Exit(runID())
		case "help":
			printUsage()
			return
		}
	}

	// "allan --resume <id>" passes the id where a subcommand would be.
	resume := flResume.set
	resumeID := flResume.id
	if resume && resumeID == "" && len(args) > 0 {
		resumeID = strings.TrimSpace(args[0])
	}

	if *flVersion {
		fmt.Printf("allan %s\n", version)
		return
	}

	ctx := context.Background()
	rt, err := bootstrap.Build(ctx, bootstrap.Options{
		Workspace:      *flWorkspace,
		NoMemory:       *flNoMemory,
		Backend:        *flBackend,
		Model:          *flModel,
		PickLocalModel: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if rt.Created {
		path, _ := config.ConfigPath()
		fmt.Printf("Создан дефолтный конфиг: %s\n", path)
	}
	startedAt := time.Now()
	model := tui.New(rt.Cfg, rt.Agent, rt.Workspace, version)
	if rt.Notice != "" {
		model.Notice(rt.Notice)
	}
	if resume {
		msgs, err := rt.Resume(ctx, resumeID, 200)
		if err != nil {
			fmt.Fprintf(os.Stderr, "--resume: %v\n", err)
		} else {
			model.Restore(msgs)
			fmt.Printf("Продолжена сессия %s: сообщений %d, модель %s/%s\n",
				rt.Agent.SessionID, len(msgs), rt.Cfg.Backend.Type, rt.Cfg.Backend.Model)
		}
	}
	if err := tui.Run(model); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
	}

	// On exit:
	rt.Close(ctx)
	printSummary(rt.Agent, startedAt)
}

func printSummary(ag *agent.Agent, started time.Time) {
	dur := time.Since(started).Round(time.Second)
	fmt.Println()
	fmt.Printf("◆ allan · сессия завершена за %s\n", dur)
	fmt.Printf("  модель       %s/%s\n", ag.Cfg.Backend.Type, ag.Cfg.Backend.Model)
	fmt.Printf("  инструменты  %d (✓ %d  ✗ %d)\n", ag.Stats.ToolCalls, ag.Stats.SuccessCalls, ag.Stats.FailedCalls)
	fmt.Printf("  токены       %d вход · %d выход\n", ag.Stats.TokensIn, ag.Stats.TokensOut)
	fmt.Printf("  сессия       %s (продолжить: allan --resume %s)\n", ag.SessionID, ag.SessionID)
}

func printUsage() {
	fmt.Print(i18n.F(usageText, version, strings.Join(knownProviders(), ", ")))
	// Which computer this is: with several PCs on one account the app labels
	// them by name, and the id tells two machines with the same name apart.
	fmt.Print(i18n.F("\nЭта машина: %s · id %s  (подробнее: allan id)\n", link.MachineName(), link.ShortID(link.MachineID())))
}

const usageText = `Allan %s — автономный агент в терминале

Использование:
  allan [флаги]                 запустить TUI в текущей папке
  allan key set <provider>      сохранить API-ключ (скрытый ввод, системный keyring)
  allan key list | rm <provider>
  allan connect [код]           связать эту машину с KEYTRON Prime (страница выдаёт код)
  allan serve                   HTTP-воркер для телефона и сайта (обычно в фоне)
  allan id                      имя, уникальный id и адрес этой машины
  allan help                    эта справка

Флаги:
  --model <имя>          модель на этот запуск (например glm-5.2:cloud)
  --backend <провайдер>  провайдер на этот запуск: ollama, openrouter, opencode, ...
  --workspace <папка>    рабочая папка агента (по умолчанию текущая)
  --resume [id]          продолжить последнюю сессию или конкретную по id
  --no-memory            не читать и не писать память
  --version              показать версию

Провайдеры:
  облачные   %s
  локальные  ollama, llamacpp, lmstudio (без ключа, находятся автоматически)

Внутри TUI: /help — команды, /model — выбрать модель, /api — ключи и эндпоинты,
/connect — связать машину с KEYTRON Prime.

Файлы:
  ~/.allan/config.toml   настройки (без ключей)
  ~/.allan/memory.db     память и сессии
  ключи                  системный keyring, иначе ~/.allan/secrets.json (0600)

Примеры:
  allan key set openrouter
  allan connect                       # печатает ссылку и ждёт код со страницы
  allan serve --listen 127.0.0.1:8790
  allan --backend opencode --model deepseek-v4-flash-free
  allan --workspace ~/git/project --resume
`
