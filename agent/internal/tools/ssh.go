package tools

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SSHTool struct {
	StrictHostKeys bool
	DefaultTimeout int
}

func NewSSHTool(strict bool, timeout int) *SSHTool {
	if timeout <= 0 {
		timeout = 30
	}
	return &SSHTool{StrictHostKeys: strict, DefaultTimeout: timeout}
}

func (s *SSHTool) Name() string { return "ssh" }

func (s *SSHTool) Description() string {
	return "Run a command on a remote host via SSH using key-based authentication. Reads ~/.ssh/known_hosts and ~/.ssh/id_* keys. Password auth is not supported."
}

func (s *SSHTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"host":    map[string]any{"type": "string", "description": "Hostname or IP"},
			"command": map[string]any{"type": "string", "description": "Command to run"},
			"port":    map[string]any{"type": "integer", "description": "SSH port (default 22)"},
			"user":    map[string]any{"type": "string", "description": "Username (default: current user)"},
			"timeout": map[string]any{"type": "integer", "description": "Timeout seconds (default 30)"},
		},
		"required": []string{"host", "command"},
	}
}

func (s *SSHTool) authMethods() ([]ssh.AuthMethod, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	keys := []string{"id_ed25519", "id_rsa", "id_ecdsa"}
	var signers []ssh.Signer
	for _, k := range keys {
		path := filepath.Join(home, ".ssh", k)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			continue
		}
		signers = append(signers, signer)
	}
	if len(signers) == 0 {
		return nil, fmt.Errorf("no usable SSH private keys found in ~/.ssh")
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil
}

func (s *SSHTool) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if !s.StrictHostKeys {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(khPath); err != nil {
		return nil, fmt.Errorf("known_hosts not found: %w", err)
	}
	return knownhosts.New(khPath)
}

func (s *SSHTool) Run(ctx context.Context, params map[string]any) (string, error) {
	host, _ := params["host"].(string)
	command, _ := params["command"].(string)
	if host == "" || command == "" {
		return "", fmt.Errorf("host and command are required")
	}
	port := 22
	if v, ok := params["port"].(float64); ok {
		port = int(v)
	}
	user, _ := params["user"].(string)
	if user == "" {
		if u := os.Getenv("USER"); u != "" {
			user = u
		} else {
			user = "root"
		}
	}
	timeout := s.DefaultTimeout
	if v, ok := params["timeout"].(float64); ok {
		timeout = int(v)
	}

	auth, err := s.authMethods()
	if err != nil {
		return "", err
	}
	hkcb, err := s.hostKeyCallback()
	if err != nil {
		return "", err
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: hkcb,
		Timeout:         time.Duration(timeout) * time.Second,
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()
	exitCode := 0
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(time.Duration(timeout) * time.Second):
		session.Signal(ssh.SIGKILL)
		return "", fmt.Errorf("ssh command timed out")
	case err := <-done:
		if err != nil {
			if ee, ok := err.(*ssh.ExitError); ok {
				exitCode = ee.ExitStatus()
			} else {
				return "", err
			}
		}
	}
	out := stdout.String()
	if stderr.Len() > 0 {
		out += "\n[stderr]\n" + stderr.String()
	}
	out += fmt.Sprintf("\n[exit %d]", exitCode)
	return out, nil
}
