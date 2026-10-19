// Package audiosetup installs local speech engines for `allan audio setup`:
// the on-device macOS dictation helper, faster-whisper (whisper-ctranslate2)
// and Piper voices. Everything goes to the user's home, nothing needs root.
package audiosetup

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/keytron/allan/agent/internal/serve"
)

//go:embed AllanSpeech.swift
var swiftSource []byte

//go:embed Info.plist
var infoPlist []byte

// PiperVoices are downloaded from rhasspy/piper-voices: one Russian and one English voice.
var PiperVoices = []struct{ Name, Path string }{
	{"ru_RU-irina-medium", "ru/ru_RU/irina/medium/ru_RU-irina-medium"},
	{"en_US-amy-medium", "en/en_US/amy/medium/en_US-amy-medium"},
}

const piperBase = "https://huggingface.co/rhasspy/piper-voices/resolve/main/"

// CanBuildMacHelper tells whether this Mac has the Swift compiler (Xcode Command Line Tools).
func CanBuildMacHelper() bool {
	_, err := exec.LookPath("swiftc")
	return runtime.GOOS == "darwin" && err == nil
}

// BuildMacHelper compiles the dictation helper app into ~/.allan/helpers and
// asks macOS for the speech recognition permission (a system dialog appears once).
func BuildMacHelper(log io.Writer) (granted bool, err error) {
	home, _ := os.UserHomeDir()
	app := filepath.Join(home, ".allan", "helpers", "AllanSpeech.app")
	bin := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return false, err
	}
	src := filepath.Join(os.TempDir(), "AllanSpeech.swift")
	if err := os.WriteFile(src, swiftSource, 0o644); err != nil {
		return false, err
	}
	defer os.Remove(src)
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), infoPlist, 0o644); err != nil {
		return false, err
	}
	if err := run(log, "swiftc", "-O", "-o", filepath.Join(bin, "AllanSpeech"), src); err != nil {
		return false, err
	}
	// ad-hoc signature: macOS ties the permission to the code signature
	if err := run(log, "codesign", "--force", "--sign", "-", app); err != nil {
		return false, err
	}
	out := filepath.Join(os.TempDir(), "allan-speech-auth.txt")
	_ = os.Remove(out + ".err")
	cmd := exec.Command("open", "-W", "-n", "-a", app)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		return false, err
	}
	return serve.AppleDictationApp() != "", nil
}

// InstallFasterWhisper puts whisper-ctranslate2 (faster-whisper CLI) into the user's tools.
func InstallFasterWhisper(log io.Writer) error {
	return installTool(log, "whisper-ctranslate2")
}

// InstallPiper installs the Piper CLI and downloads the default voices.
func InstallPiper(log io.Writer) error {
	if err := installTool(log, "piper-tts"); err != nil {
		return err
	}
	dir := serve.PiperVoiceDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, v := range PiperVoices {
		for _, ext := range []string{".onnx", ".onnx.json"} {
			dst := filepath.Join(dir, v.Name+ext)
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			fmt.Fprintf(log, "  ↓ %s%s\n", v.Name, ext)
			if err := download(piperBase+v.Path+ext+"?download=true", dst); err != nil {
				return err
			}
		}
	}
	return nil
}

func installTool(log io.Writer, pkg string) error {
	switch {
	case have("uv"):
		return run(log, "uv", "tool", "install", "--upgrade", pkg)
	case have("pipx"):
		return run(log, "pipx", "install", "--force", pkg)
	}
	return fmt.Errorf("нужен uv или pipx, чтобы поставить %s без root", pkg)
}

func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func run(log io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	return nil
}

func download(url, dst string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
