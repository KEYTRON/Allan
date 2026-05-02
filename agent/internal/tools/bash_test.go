package tools

import "testing"

func TestIsDangerous(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"rm -rf /", true},
		{"rm -rf /*", true},
		{"sudo rm -rf /", true},
		{"rm -rf /home/user/scratch", false},
		{"echo hi", false},
		{":(){ :|:& };:", true},
	}
	for _, c := range cases {
		if got := IsDangerous(c.cmd); got != c.want {
			t.Errorf("IsDangerous(%q) = %v, want %v", c.cmd, got, c.want)
		}
	}
}

func TestIsInteractive(t *testing.T) {
	if !IsInteractive("sudo apt update") {
		t.Error("sudo should be interactive")
	}
	if !IsInteractive("vim file.go") {
		t.Error("vim should be interactive")
	}
	if IsInteractive("ls -la") {
		t.Error("ls should not be interactive")
	}
}
