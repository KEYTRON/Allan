package main

import (
	"bufio"
	"fmt"
	"github.com/keytron/allan/agent/internal/i18n"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/keytron/allan/agent/config"
)

// The worker is meant to live in the background: started from a terminal,
// `allan serve` re-runs itself detached with --foreground, writes the pid and
// a log next to the config and gives the terminal back. Init systems
// (systemd, OpenRC, launchd, runit) run it without a terminal, and then it
// stays in the foreground as they expect.

func servePaths() (pidPath, logPath string, err error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "serve.pid"), filepath.Join(dir, "serve.log"), nil
}

// runningPid is the pid of a live background worker, or 0.
func runningPid() int {
	pidPath, _, err := servePaths()
	if err != nil {
		return 0
	}
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 || !alive(pid) {
		return 0
	}
	return pid
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// onTerminal tells whether a person started us (detach) or an init system (stay).
func onTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func serveBackground(args []string) int {
	if pid := runningPid(); pid != 0 {
		fmt.Print(i18n.F("allan serve уже работает в фоне (pid %d). Остановить: allan serve stop\n", pid))
		return 0
	}
	pidPath, logPath, err := servePaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer logf.Close()
	start, _ := logf.Seek(0, 2)
	cmd := exec.Command(self, append([]string{"serve", "--foreground"}, args...)...)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = detachAttr()
	if err := cmd.Start(); err != nil {
		fmt.Fprint(os.Stderr, i18n.F("не удалось запустить воркер: %v\n", err))
		return 1
	}
	pid := cmd.Process.Pid
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o600)
	_ = cmd.Process.Release()

	// give it a moment: a busy port or a missing token show up right away
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		out := logSince(logPath, start)
		if !alive(pid) {
			_ = os.Remove(pidPath)
			fmt.Fprint(os.Stderr, out)
			fmt.Fprintln(os.Stderr, i18n.S("воркер не запустился, подробности выше"))
			return 1
		}
		if strings.Contains(out, "туннель: на связи") || strings.Contains(out, "tunnel: connected") {
			break
		}
	}
	fmt.Print(logSince(logPath, start))
	fmt.Print(i18n.F("\nallan serve работает в фоне (pid %d)\n  лог        %s\n  состояние  allan serve status\n  остановить allan serve stop\n", pid, logPath))
	return 0
}

func logSince(path string, offset int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return ""
	}
	var sb strings.Builder
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sb.WriteString(sc.Text() + "\n")
	}
	return sb.String()
}

func serveStop() int {
	pid := runningPid()
	if pid == 0 {
		fmt.Println(i18n.S("фоновый allan serve не запущен"))
		return 0
	}
	p, _ := os.FindProcess(pid)
	if err := p.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprint(os.Stderr, i18n.F("не удалось остановить pid %d: %v\n", pid, err))
		return 1
	}
	for i := 0; i < 50 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if pidPath, _, err := servePaths(); err == nil {
		_ = os.Remove(pidPath)
	}
	fmt.Print(i18n.F("allan serve остановлен (pid %d)\n", pid))
	return 0
}

func serveStatus() int {
	_, logPath, _ := servePaths()
	pid := runningPid()
	if pid == 0 {
		fmt.Println(i18n.S("фоновый allan serve не запущен. Запустить: allan serve"))
		return 1
	}
	fmt.Print(i18n.F("allan serve работает в фоне (pid %d), лог %s\n\n", pid, logPath))
	fmt.Print(tail(logPath, 8))
	return 0
}

func serveLogs() int {
	_, logPath, _ := servePaths()
	out := tail(logPath, 60)
	if out == "" {
		fmt.Println("лог пуст: " + logPath)
		return 0
	}
	fmt.Print(out)
	return 0
}

func tail(path string, n int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n"
}
