package engine

import (
	"errors"
	"strings"

	"github.com/yang-bin-free/claude-phone/pkg/desktop"
)

const (
	commandMaxWords = 8
	commandMaxBytes = 200
)

// ValidateCommandSetting validates a user-entered CLI command. Empty means
// "use the default". Non-empty commands are whitespace-split, must not
// contain quotes, must stay within size limits, and their first word must
// resolve to an executable through the provided resolver.
func ValidateCommandSetting(command, displayName string, resolve func(string) (string, error)) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil
	}
	if strings.ContainsAny(command, "\"'") {
		return "", errors.New(displayName + " 命令不支持引号参数")
	}
	if strings.ContainsAny(command, "\t\n\r\v\f") {
		return "", errors.New(displayName + " 命令包含非法空白字符")
	}
	if len(command) > commandMaxBytes {
		return "", errors.New(displayName + " 命令超过长度限制")
	}
	words := strings.Fields(command)
	if len(words) > commandMaxWords {
		return "", errors.New(displayName + " 命令参数过多")
	}
	if _, err := resolve(words[0]); err != nil {
		return "", errors.New(displayName + " 命令的第一个词不是可执行文件")
	}
	return command, nil
}

// desktopResolveWord resolves a single executable word through the desktop
// CLI search paths. Package var so tests can stub it.
var desktopResolveWord = func(name string) (string, error) {
	return desktop.ResolveCommandWord(name)
}

// validateCommand validates a settings command with the production resolver.
func (e *Engine) validateCommand(command, displayName string) (string, error) {
	return ValidateCommandSetting(command, displayName, desktopResolveWord)
}
