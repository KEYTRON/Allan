package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/keytron/allan/agent/config"
	"github.com/keytron/allan/agent/internal/backend"
	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/link"
)

const oauthDone = `<!doctype html><meta charset="utf-8"><title>Allan</title>
<body style="font-family:sans-serif;text-align:center;padding:3em">%s</body>`

// loginOpenRouter signs in on openrouter.ai and stores the key it issues, so
// the user never copies a key by hand. With a desktop the browser returns to
// a one-off listener on localhost; over SSH, or when the browser cannot reach
// this machine, the user pastes the code (or the whole address) instead.
func loginOpenRouter(cfg *config.Config) int {
	pkce, err := backend.NewPKCE()
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.F("не удалось начать вход: %v\n", err))
		return 1
	}
	label := "Allan " + link.MachineName()
	codes := make(chan string, 2)

	callback := ""
	if !remoteSession() {
		if ln, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
			callback = fmt.Sprintf("http://localhost:%d/callback", ln.Addr().(*net.TCPAddr).Port)
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				code := r.URL.Query().Get("code")
				if code == "" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprintf(w, oauthDone, i18n.S("Готово, вернитесь в терминал."))
				codes <- code
			})}
			go func() { _ = srv.Serve(ln) }()
			defer srv.Close()
		}
	}

	authLink := backend.OpenRouterAuthLink(pkce, callback, label)
	fmt.Println(i18n.S("Войдите в OpenRouter и разрешите доступ:"))
	fmt.Println("  " + authLink)
	if callback != "" {
		openBrowser(authLink)
		fmt.Println(i18n.S("Жду ответа браузера. Если он открыт на другой машине, вставьте сюда адрес, на который он перешёл:"))
	} else {
		fmt.Println(i18n.S("После входа страница покажет код — вставьте его сюда:"))
	}
	go func() {
		if line, err := readLine(); err == nil {
			codes <- backend.CodeFromInput(line)
		}
	}()

	var code string
	select {
	case code = <-codes:
	case <-time.After(10 * time.Minute):
		fmt.Fprintln(os.Stderr, i18n.S("время на вход вышло (10 минут)"))
		return 1
	}
	if code == "" {
		fmt.Fprintln(os.Stderr, i18n.S("пустой код, ничего не сохранено"))
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	key, err := backend.ExchangeOpenRouterCode(ctx, code, pkce)
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.F("OpenRouter не выдал ключ: %v\n", err))
		return 1
	}
	return saveKey(cfg, "openrouter", key)
}

// remoteSession is true over SSH without a forwarded display: a browser there
// cannot reach this machine's localhost.
func remoteSession() bool {
	ssh := os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""
	return ssh && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "android":
		cmd = exec.Command("termux-open-url", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	cmd.Stdout, cmd.Stderr = nil, nil
	_ = cmd.Start()
}
