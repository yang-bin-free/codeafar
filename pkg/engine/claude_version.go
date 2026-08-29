package engine

import (
	"errors"
	"os/exec"
	"regexp"
)

var cliVersionPattern = regexp.MustCompile(`\b(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)\b`)
var claudeVersionPattern = cliVersionPattern

func DetectClaudeVersion(bin string) (string, error) {
	if bin == "" {
		bin = "claude"
	}
	return DetectCLIVersion([]string{bin}, "Claude")
}

// DetectCLIVersion runs `<command> --version` and extracts the semantic
// version. command[0] is the executable; remaining words are prepend args for
// wrapper CLIs.
func DetectCLIVersion(command []string, productName string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("empty " + productName + " command")
	}
	args := append(append([]string(nil), command[1:]...), "--version")
	output, err := exec.Command(command[0], args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	match := cliVersionPattern.FindSubmatch(output)
	if len(match) != 2 {
		return "", errors.New("unable to parse " + productName + " CLI version")
	}
	return string(match[1]), nil
}
