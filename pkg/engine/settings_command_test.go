package engine

import (
	"errors"
	"strings"
	"testing"
)

func fakeResolve(ok bool) func(string) (string, error) {
	return func(string) (string, error) {
		if ok {
			return "/resolved/bin", nil
		}
		return "", errors.New("not found")
	}
}

func TestValidateCommandSettingAcceptsEmpty(t *testing.T) {
	got, err := ValidateCommandSetting("", "Claude", fakeResolve(true))
	if err != nil || got != "" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestValidateCommandSettingAcceptsResolvable(t *testing.T) {
	got, err := ValidateCommandSetting("wrapper claude", "Claude", fakeResolve(true))
	if err != nil || got != "wrapper claude" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestValidateCommandSettingRejectsQuotes(t *testing.T) {
	for _, bad := range []string{`wrapper "a b"`, "wrapper 'x'"} {
		if _, err := ValidateCommandSetting(bad, "Claude", fakeResolve(true)); err == nil {
			t.Fatalf("expected quote rejection for %q", bad)
		}
	}
}

func TestValidateCommandSettingRejectsTooManyWords(t *testing.T) {
	words := make([]string, 9)
	for i := range words {
		words[i] = "w"
	}
	if _, err := ValidateCommandSetting(strings.Join(words, " "), "Claude", fakeResolve(true)); err == nil {
		t.Fatal("expected word-count rejection")
	}
}

func TestValidateCommandSettingRejectsTooLong(t *testing.T) {
	long := strings.Repeat("a", 201)
	if _, err := ValidateCommandSetting(long, "Claude", fakeResolve(true)); err == nil {
		t.Fatal("expected length rejection")
	}
}

func TestValidateCommandSettingRejectsUnresolvable(t *testing.T) {
	if _, err := ValidateCommandSetting("nosuch bin", "Claude", fakeResolve(false)); err == nil {
		t.Fatal("expected resolution rejection")
	}
}
