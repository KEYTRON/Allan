package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/keytron/allan/agent/internal/i18n"
	"github.com/keytron/allan/agent/internal/link"
)

const connectUsage = `allan connect — связать эту машину с KEYTRON Prime

  allan connect              показать страницу и принять код оттуда
  allan connect <код>        сразу обменять код на токен (для скриптов)
  allan connect status       что сейчас подключено
  allan connect url          только ссылка на страницу привязки
  allan connect reset        забыть сервер и удалить токен

Флаги:
  --server <url>   адрес KEYTRON Prime (по умолчанию %s, или ALLAN_SERVER_URL)

Как это работает: страница выдаёт одноразовый код после входа на сайт,
эта команда обменивает код на общий токен и кладёт его в системный keyring.
Дальше телефон и сайт обращаются к агенту через сервер с этим токеном.
`

func runConnect(args []string) int {
	var serverURL string
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server":
			if i+1 < len(args) {
				serverURL = args[i+1]
				i++
			}
		case "-h", "--help", "help":
			fmt.Print(i18n.F(connectUsage, link.DefaultServer))
			return 0
		default:
			positional = append(positional, args[i])
		}
	}

	sub, code := "pair", ""
	if len(positional) > 0 {
		sub = strings.ToLower(positional[0])
	}
	if len(positional) > 1 {
		code = positional[1]
	}

	switch sub {
	case "url":
		fmt.Println(link.ServerURL(serverURL))
		return 0
	case "status":
		st, token, err := link.Load()
		if err != nil {
			fmt.Println(link.Summary(nil, ""))
			fmt.Println("Запустите: allan connect")
			return 1
		}
		fmt.Println(link.Summary(st, token))
		if token == "" {
			return 1
		}
		fmt.Println()
		fmt.Println("Запустить воркер:  allan serve")
		return 0
	case "reset", "forget":
		st, _, _ := link.Load()
		if err := link.Reset(); err != nil {
			fmt.Fprintf(os.Stderr, "не удалось сбросить: %v\n", err)
			return 1
		}
		if st != nil {
			fmt.Printf("Связь с %s удалена, токен воркера стёрт.\n", st.Server)
		} else {
			fmt.Println("Нечего сбрасывать.")
		}
		return 0
	case "pair", "start":
		return connectPair(serverURL, code)
	default:
		// `allan connect <код>` without a subcommand
		return connectPair(serverURL, sub)
	}
}

func connectPair(serverURL, code string) int {
	base := link.ServerURL(serverURL)
	if strings.TrimSpace(code) == "" {
		fmt.Print(link.Instructions(base))
		fmt.Print("Код со страницы: ")
		line, err := readLine()
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ввод кода: %v\n", err)
			return 1
		}
		code = strings.TrimSpace(line)
	}
	if code == "" {
		fmt.Fprintln(os.Stderr, "пустой код, ничего не подключено")
		return 1
	}

	st, token, err := link.Exchange(context.Background(), base, code, link.Meta{Version: version})
	if err != nil {
		fmt.Fprintf(os.Stderr, "не получилось обменять код: %v\n", err)
		fmt.Fprintf(os.Stderr, "Проверьте, что код ещё действителен и вы вошли на %s\n", base)
		return 1
	}
	if err := link.Save(st, token); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Println()
	fmt.Printf("Машина связана с %s\n", st.Server)
	if st.Name != "" {
		fmt.Printf("  имя       %s\n", st.Name)
	}
	if st.WorkerID != "" {
		fmt.Printf("  worker id %s\n", st.WorkerID)
	}
	fmt.Printf("  токен     %s (в системном keyring)\n", link.MaskToken(token))
	fmt.Println()
	fmt.Println("Теперь запустите воркер, чтобы телефон и сайт до него достучались:")
	fmt.Println("  allan serve")
	return 0
}

func readLine() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
