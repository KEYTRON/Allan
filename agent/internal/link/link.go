// Package link pairs this machine with a KEYTRON Prime install.
//
// The flow mirrors how a phone or a browser would do it: the site issues a
// one-time code, the machine exchanges that code for a shared worker token and
// keeps it in the system keyring. The token is the only thing that makes the
// agent reachable, so it never lands in a config file or in shell history.
package link

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/keytron/allan/agent/config"
)

// DefaultServer is where the pairing page lives.
const DefaultServer = "https://allan.keytron-prime.org"

// TokenName is the keyring entry that holds the worker token.
const TokenName = "keytron-worker"

// State is the non-secret half of the pairing, stored in ~/.allan/connection.json.
type State struct {
	Server   string `json:"server"`
	WorkerID string `json:"worker_id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Allan    string `json:"allan"`
	// MachineID and Address are what the site was told at pairing time.
	MachineID string    `json:"machine_id,omitempty"`
	Address   string    `json:"address,omitempty"`
	PairedAt  time.Time `json:"paired_at"`
}

// Meta is what the site shows next to the machine it was paired with.
type Meta struct {
	Host     string
	Platform string
	Version  string
	// MachineID is the stable id from MachineID(); the site uses it to tell the
	// same PC connecting again from a second PC.
	MachineID string
	// Name is how the PC is shown in the app ("Mac", "gentoo-pc").
	Name string
	// Address is where the site reaches this PC's worker (Tailscale URL).
	Address string
}

// ServerURL resolves which server to talk to: explicit flag, environment, the
// server we are already paired with, then the default.
func ServerURL(override string) string {
	if s := strings.TrimSpace(override); s != "" {
		return strings.TrimRight(s, "/")
	}
	if s := strings.TrimSpace(os.Getenv("ALLAN_SERVER_URL")); s != "" {
		return strings.TrimRight(s, "/")
	}
	if st, _, err := Load(); err == nil && st.Server != "" {
		return st.Server
	}
	return DefaultServer
}

// Load reads the stored pairing and the token from the secret store.
func Load() (*State, string, error) {
	path, err := statePath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, "", fmt.Errorf("connection.json повреждён: %w", err)
	}
	token, _ := config.OpenSecrets(configDir()).Get(TokenName)
	return &st, token, nil
}

// Save persists the pairing and puts the token into the secret store.
func Save(st *State, token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("пустой токен")
	}
	if err := config.OpenSecrets(configDir()).Set(TokenName, token); err != nil {
		return fmt.Errorf("сохранить токен: %w", err)
	}
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

// Reset forgets the server and deletes the token.
func Reset() error {
	_ = config.OpenSecrets(configDir()).Delete(TokenName)
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Exchange trades a one-time code from the pairing page for a worker token.
func Exchange(ctx context.Context, base, code string, meta Meta) (*State, string, error) {
	if meta.Host == "" {
		meta.Host, _ = os.Hostname()
	}
	if meta.Platform == "" {
		meta.Platform = runtime.GOOS + "/" + runtime.GOARCH
	}
	payload, err := json.Marshal(map[string]any{
		"code":       strings.TrimSpace(code),
		"host":       meta.Host,
		"platform":   meta.Platform,
		"allan":      meta.Version,
		"machine_id": meta.MachineID,
		"name":       meta.Name,
		"address":    meta.Address,
	})
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(base, "/")+"/api/allan/pair/exchange/", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return nil, "", fmt.Errorf("%s", e.Error)
		}
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		Token    string `json:"token"`
		WorkerID string `json:"worker_id"`
		Name     string `json:"name"`
		Server   string `json:"server"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(out.Token) == "" {
		return nil, "", fmt.Errorf("сервер не вернул токен")
	}
	server := out.Server
	if server == "" {
		server = strings.TrimRight(base, "/")
	}
	st := &State{
		Server:    server,
		WorkerID:  out.WorkerID,
		Name:      out.Name,
		Host:      meta.Host,
		Allan:     meta.Version,
		MachineID: meta.MachineID,
		Address:   meta.Address,
		PairedAt:  time.Now(),
	}
	return st, out.Token, nil
}

// Instructions is the text shown when pairing is needed.
func Instructions(base string) string {
	return fmt.Sprintf(`Связать эту машину с KEYTRON Prime

  1. Откройте в браузере:  %s/pair
  2. Войдите на сайт (или зарегистрируйтесь)
  3. Нажмите «Выдать код» и скопируйте код
  4. Вставьте его сюда: /connect <код>

`, strings.TrimRight(base, "/"))
}

// Summary is the human-readable pairing status.
func Summary(st *State, token string) string {
	if st == nil {
		return "Машина не связана с KEYTRON Prime."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "сервер     %s\n", st.Server)
	if st.Name != "" {
		fmt.Fprintf(&sb, "имя        %s\n", st.Name)
	}
	if st.WorkerID != "" {
		fmt.Fprintf(&sb, "worker id  %s\n", st.WorkerID)
	}
	if st.Host != "" {
		fmt.Fprintf(&sb, "хост       %s\n", st.Host)
	}
	if st.PairedAt.IsZero() {
		sb.WriteString("привязан   неизвестно когда\n")
	} else {
		fmt.Fprintf(&sb, "привязан   %s\n", st.PairedAt.Format("2006-01-02 15:04"))
	}
	if token == "" {
		sb.WriteString("токен      ОТСУТСТВУЕТ — пройдите allan connect заново\n")
	} else {
		fmt.Fprintf(&sb, "токен      %s\n", MaskToken(token))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// MaskToken keeps just enough of a token to recognise it in a list.
func MaskToken(t string) string {
	if len(t) <= 8 {
		return strings.Repeat("*", len(t))
	}
	return t[:4] + "…" + t[len(t)-4:]
}

func configDir() string {
	dir, err := config.ConfigDir()
	if err != nil {
		return ".allan"
	}
	return dir
}

func statePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "connection.json"), nil
}
