package desktop

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCommandSplitsMultiWordCommand(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "wrapper")
	t.Setenv("PATH", dir)
	got, err := ResolveCommand("wrapper claude extra", "claude", "Claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != filepath.Join(dir, "wrapper") {
		t.Fatalf("path=%q", got.Path)
	}
	if strings.Join(got.PrependArgs, " ") != "claude extra" {
		t.Fatalf("prepend=%v", got.PrependArgs)
	}
}

func TestResolveCommandSingleWordHasNoPrependArgs(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "claude")
	t.Setenv("PATH", dir)
	got, err := ResolveCommand("claude", "claude", "Claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != bin || len(got.PrependArgs) != 0 {
		t.Fatalf("got=%+v", got)
	}
}

func TestResolveCommandAbsolutePathWithArgs(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "wrapper")
	got, err := ResolveCommand(bin+" claude", "claude", "Claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != bin || len(got.PrependArgs) != 1 || got.PrependArgs[0] != "claude" {
		t.Fatalf("got=%+v", got)
	}
}

func TestResolveCommandMissingExecutableFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := ResolveCommand("nosuchwrapper claude", "claude", "Claude", false)
	if err == nil || !strings.Contains(err.Error(), "Claude") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolvedCommandString(t *testing.T) {
	c := ResolvedCommand{Path: "/bin/wrapper", PrependArgs: []string{"claude"}}
	if c.String() != "/bin/wrapper claude" {
		t.Fatalf("string=%q", c.String())
	}
	if (ResolvedCommand{Path: "/bin/claude"}).String() != "/bin/claude" {
		t.Fatal("single-word String() broken")
	}
}
