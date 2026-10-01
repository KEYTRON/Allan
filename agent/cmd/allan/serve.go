package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/keytron/allan/agent/internal/bootstrap"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/link"
	"github.com/keytron/allan/agent/internal/serve"
)

const serveUsage = `allan serve — HTTP-воркер: отдаёт агента телефону и сайту

  allan serve [флаги]

Флаги:
  --listen <addr>     адрес слушания (по умолчанию 127.0.0.1:8790)
  --token <токен>     общий секрет; иначе берётся из connect (keyring) или ALLAN_WORKER_TOKEN
  --workspace <папка> рабочая папка агента
  --max-sessions <n>  сколько сессий держать в памяти (по умолчанию 32)
  --public            слушать на всех интерфейсах (осторожно: только за TLS-прокси)

Токен хранится в системном keyring. Воркер сам никуда не ходит: он только
отвечает на запросы, поэтому держать его на локальном адресе или на Tailscale
безопасно, а вот в интернет — только за nginx с токеном на телефоне.

Примеры:
  allan serve                                   # только localhost
  allan serve --listen 100.118.140.15:8790      # Tailscale, для сервера-шлюза
  ALLAN_WORKER_TOKEN=… allan serve --listen 0.0.0.0:8790   # за обратным прокси
`

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, i18n.S(serveUsage)) }
	flListen := fs.String("listen", "127.0.0.1:8790", "адрес слушания")
	flToken := fs.String("token", "", "общий токен")
	flWorkspace := fs.String("workspace", "", "рабочая папка")
	flMax := fs.Int("max-sessions", 32, "сколько сессий держать")
	flPublic := fs.Bool("public", false, "слушать на всех интерфейсах")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	token, err := resolveWorkerToken(*flToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprint(os.Stderr, "\nСначала свяжите машину с KEYTRON Prime: allan connect\n")
		return 1
	}
	addr := *flListen
	if *flPublic && strings.HasPrefix(addr, "127.0.0.1") {
		addr = ":" + strings.TrimPrefix(addr, "127.0.0.1:")
	}

	srv, err := serve.New(serve.Options{
		Addr:     addr,
		Token:    token,
		Version:  version,
		MaxTurns: *flMax,
		Build: func(ctx context.Context) (*bootstrap.Runtime, error) {
			return bootstrap.Build(ctx, bootstrap.Options{
				Workspace:      *flWorkspace,
				PickLocalModel: true,
			})
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Fail fast on a busy port instead of dying later inside ListenAndServe.
	probe, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "не удалось занять %s: %v\n", addr, err)
		return 1
	}
	_ = probe.Close()

	fmt.Printf("◆ allan serve %s слушает %s\n", version, addr)
	fmt.Printf("  токен      %s (%s)\n", maskToken(token), tokenSource(*flToken))
	fmt.Printf("  страница   %s/pair\n", link.ServerURL(""))
	fmt.Printf("  остановить: Ctrl+C\n\n")

	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	fmt.Println("воркер остановлен")
	return 0
}

// resolveWorkerToken takes the token from the flag, the pairing done by
// `allan connect` (system keyring), or the environment.
func resolveWorkerToken(flagToken string) (string, error) {
	if t := strings.TrimSpace(flagToken); t != "" {
		return t, nil
	}
	if t := strings.TrimSpace(os.Getenv("ALLAN_WORKER_TOKEN")); t != "" {
		return t, nil
	}
	if _, token, err := link.Load(); err == nil && token != "" {
		return token, nil
	}
	return "", fmt.Errorf("нет токена воркера")
}

func tokenSource(flagToken string) string {
	switch {
	case strings.TrimSpace(flagToken) != "":
		return "из --token"
	case strings.TrimSpace(os.Getenv("ALLAN_WORKER_TOKEN")) != "":
		return "из ALLAN_WORKER_TOKEN"
	default:
		return "из allan connect"
	}
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return strings.Repeat("*", len(t))
	}
	return t[:4] + "…" + t[len(t)-4:]
}
