package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ResolvedCommand is a resolved CLI command: the executable path plus any
// leading arguments from a multi-word command such as "wrapper claude".
type ResolvedCommand struct {
	Path        string
	PrependArgs []string
}

// String renders the command as it would be typed on a command line.
func (c ResolvedCommand) String() string {
	return strings.Join(append([]string{c.Path}, c.PrependArgs...), " ")
}

// ResolveCommand resolves a requested CLI command, which may be multi-word
// (for example "wrapper claude"). The first word is resolved to an executable
// path and the remaining words become PrependArgs. An empty requested value
// falls back to defaultName.
func ResolveCommand(requested, defaultName, displayName string, includeClaudeLocal bool) (ResolvedCommand, error) {
	words := strings.Fields(strings.TrimSpace(requested))
	if len(words) == 0 {
		words = []string{defaultName}
	}
	path, err := resolveCodingAgentBinary(words[0], defaultName, displayName, includeClaudeLocal)
	if err != nil {
		return ResolvedCommand{}, err
	}
	return ResolvedCommand{Path: path, PrependArgs: append([]string(nil), words[1:]...)}, nil
}

// ResolveCommandWord resolves a single executable word through the standard
// CLI search paths.
func ResolveCommandWord(requested string) (string, error) {
	return resolveCodingAgentBinary(requested, requested, requested, false)
}

// resolveCodingAgentBinary finds a runnable executable for a single-word
// command name even when the app was launched from Finder with macOS's
// minimal PATH.
func resolveCodingAgentBinary(requested, defaultName, displayName string, includeClaudeLocal bool) (string, error) {
	if strings.TrimSpace(requested) == "" {
		requested = defaultName
	}
	searched := make([]string, 0, 6)
	seen := map[string]bool{}
	add := func(path string) {
		if path != "" && !seen[path] {
			seen[path] = true
			searched = append(searched, path)
		}
	}

	if strings.ContainsRune(requested, filepath.Separator) {
		path, err := filepath.Abs(requested)
		if err == nil {
			add(path)
			if executableFile(path) {
				return path, nil
			}
		}
		return "", fmt.Errorf("%s CLI is not executable; searched: %s", displayName, strings.Join(searched, ", "))
	}

	if path, err := exec.LookPath(requested); err == nil {
		if absolute, absErr := filepath.Abs(path); absErr == nil {
			path = absolute
		}
		add(path)
		if executableFile(path) {
			return path, nil
		}
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", requested),
		filepath.Join(home, ".volta", "bin", requested),
		filepath.Join(home, ".asdf", "shims", requested),
		filepath.Join(home, ".local", "share", "mise", "shims", requested),
	}
	if includeClaudeLocal {
		candidates = append([]string{filepath.Join(home, ".claude", "local", requested)}, candidates...)
	}
	nvmPattern := filepath.Join(home, ".nvm", "versions", "node", "*", "bin", requested)
	nvmCandidates, _ := filepath.Glob(nvmPattern)
	sort.Sort(sort.Reverse(sort.StringSlice(nvmCandidates)))
	if len(nvmCandidates) == 0 {
		candidates = append(candidates, nvmPattern)
	} else {
		candidates = append(candidates, nvmCandidates...)
	}
	candidates = append(candidates,
		"/opt/homebrew/bin/"+requested,
		"/usr/local/bin/"+requested,
	)
	for _, path := range candidates {
		add(path)
		if executableFile(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s CLI was not found; searched: %s", displayName, strings.Join(searched, ", "))
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
