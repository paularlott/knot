package agent_client

import (
	"os/exec"
	"testing"
)

func TestBuildShellCommandKeepsArguments(t *testing.T) {
	args := []string{"-c", `printf '%s|' "$0" "$@"`, "a b", "it's", "", `back\slash`, "$HOME", "*"}
	script := buildShellCommand("sh", args)
	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("running %q: %v", script, err)
	}
	want := `a b|it's||back\slash|$HOME|*|`
	if string(out) != want {
		t.Fatalf("got %q, want %q (script %q)", out, want, script)
	}
}

func TestBuildShellCommandLeavesCommandString(t *testing.T) {
	out, err := exec.Command("sh", "-c", buildShellCommand("echo one | tr o 0", nil)).Output()
	if err != nil || string(out) != "0ne\n" {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestShellQuotePlainWords(t *testing.T) {
	for _, s := range []string{"-la", "/tmp/x.txt", "--dangerously-skip-permissions", "key=value"} {
		if got := shellQuote(s); got != s {
			t.Fatalf("shellQuote(%q) = %q, want it unchanged", s, got)
		}
	}
}
