package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/yang-bin-free/claude-phone/pkg/protocol"
)

const (
	codexMaxLineBytes   = 4 * 1024 * 1024
	codexMaxStderrBytes = 16 * 1024
	codexMaxErrorBytes  = 2 * 1024
)

type CodexConfig struct {
	Bin               string
	BinArgs           []string // BinArgs are prepend args for wrapper commands.
	Cwd               string
	ProviderSessionID string
	Permission        string
	Model             string
	AddDirs           []string
}

// CodexProc drives stable `codex exec --json` turns for one CodeAfar session.
type CodexProc struct {
	cfg CodexConfig

	mu                sync.Mutex
	onOut             OutputFunc
	active            *exec.Cmd
	started           bool
	stopped           bool
	providerSessionID string
}

func NewCodexProc(cfg CodexConfig) *CodexProc {
	if strings.TrimSpace(cfg.Bin) == "" {
		cfg.Bin = "codex"
	}
	return &CodexProc{cfg: cfg, providerSessionID: cfg.ProviderSessionID}
}

func (p *CodexProc) buildArgs(prompt string) ([]string, error) {
	p.mu.Lock()
	providerSessionID := p.providerSessionID
	p.mu.Unlock()
	return p.buildArgsFor(prompt, providerSessionID)
}

func (p *CodexProc) buildArgsFor(prompt, providerSessionID string) ([]string, error) {
	sandbox, ok := map[string]string{
		"readOnly":       "read-only",
		"workspaceWrite": "workspace-write",
		"fullAccess":     "danger-full-access",
	}[p.cfg.Permission]
	if !ok {
		return nil, fmt.Errorf("unsupported Codex permission %q", p.cfg.Permission)
	}
	args := []string{"-C", p.cfg.Cwd, "-s", sandbox, "-a", "never"}
	if p.cfg.Model != "" {
		args = append(args, "-m", p.cfg.Model)
	}
	for _, dir := range p.cfg.AddDirs {
		if strings.TrimSpace(dir) != "" && dir != p.cfg.Cwd {
			args = append(args, "--add-dir", dir)
		}
	}
	args = append(args, "exec")
	if providerSessionID != "" {
		args = append(args, "resume")
	}
	args = append(args, "--json")
	if providerSessionID == "" {
		args = append(args, "--color", "never")
	}
	args = append(args, "--skip-git-repo-check")
	if providerSessionID != "" {
		args = append(args, providerSessionID)
	}
	return append(args, prompt), nil
}

func (p *CodexProc) OnOutput(fn OutputFunc) {
	p.mu.Lock()
	p.onOut = fn
	p.mu.Unlock()
}

func (p *CodexProc) ProviderSessionID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.providerSessionID
}

func (p *CodexProc) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return errors.New("Codex process driver already started")
	}
	if p.stopped {
		return errors.New("Codex process driver is stopped")
	}
	if _, err := p.buildArgsFor("", p.providerSessionID); err != nil {
		return err
	}
	p.started = true
	return nil
}

func (p *CodexProc) Send(prompt string) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return errors.New("Codex process driver not started")
	}
	if p.stopped {
		p.mu.Unlock()
		return errors.New("Codex process driver is stopped")
	}
	if p.active != nil {
		p.mu.Unlock()
		return errors.New("Codex turn already running")
	}
	args, err := p.buildArgsFor(prompt, p.providerSessionID)
	if err != nil {
		p.mu.Unlock()
		return err
	}
	cmd := exec.Command(p.cfg.Bin, append(append([]string(nil), p.cfg.BinArgs...), args...)...)
	cmd.Dir = p.cfg.Cwd
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		p.mu.Unlock()
		return err
	}
	p.active = cmd
	p.mu.Unlock()
	go p.readTurn(cmd, stdout, stderr)
	return nil
}

func (p *CodexProc) readTurn(cmd *exec.Cmd, stdout, stderr io.ReadCloser) {
	stderrDone := make(chan string, 1)
	go func() { stderrDone <- readBoundedStderr(stderr) }()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), codexMaxLineBytes)
	var terminal []byte
	for scanner.Scan() {
		line := bytes.Clone(scanner.Bytes())
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
		}
		if json.Unmarshal(line, &event) == nil {
			if event.Type == "thread.started" && event.ThreadID != "" {
				p.mu.Lock()
				p.providerSessionID = event.ThreadID
				p.mu.Unlock()
			}
			if event.Type == "turn.completed" || event.Type == "turn.failed" {
				terminal = line
				continue
			}
		}
		p.emit(line)
	}
	scanErr := scanner.Err()
	if scanErr != nil {
		_, _ = io.Copy(io.Discard, stdout)
	}
	waitErr := cmd.Wait()
	stderrText := <-stderrDone

	p.mu.Lock()
	stopped := p.stopped
	if p.active == cmd {
		p.active = nil
	}
	p.mu.Unlock()
	if stopped {
		return
	}
	if terminal != nil {
		p.emit(terminal)
		return
	}
	if scanErr != nil {
		p.emitFailure("无法读取 Codex 输出: " + scanErr.Error())
		return
	}
	if waitErr != nil {
		p.emitFailure(classifyCodexFailure(stderrText, waitErr))
		return
	}
	p.emitFailure("Codex ended without a terminal event")
}

func classifyCodexFailure(stderr string, waitErr error) string {
	message := strings.ToLower(stderr)
	switch {
	case strings.Contains(message, "unexpected argument"),
		strings.Contains(message, "unknown option"),
		strings.Contains(message, "unrecognized option"),
		strings.Contains(message, "usage: codex"):
		return "Installed Codex CLI is incompatible with CodeAfar. Update Codex CLI and try again."
	case strings.Contains(message, "session") && strings.Contains(message, "not found"),
		strings.Contains(message, "thread") && strings.Contains(message, "not found"),
		strings.Contains(message, "no conversation found"),
		strings.Contains(message, "invalid session"):
		return "The saved Codex conversation could not be resumed. Create a new CodeAfar session."
	case strings.Contains(message, "not logged in"),
		strings.Contains(message, "authentication"),
		strings.Contains(message, "unauthorized"),
		strings.Contains(message, "codex login"):
		return "Codex authentication failed. Run `codex login` in Terminal and try again."
	case strings.Contains(message, "timed out"),
		strings.Contains(message, "connection refused"),
		strings.Contains(message, "network is unreachable"),
		strings.Contains(message, "could not connect"):
		return "Codex network connection failed. Check the network and try again."
	default:
		if waitErr != nil {
			return "Codex process failed: " + waitErr.Error()
		}
		return "Codex process failed."
	}
}

func (p *CodexProc) Stop() error {
	p.mu.Lock()
	p.stopped = true
	cmd := p.active
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
	}
	return nil
}

func (p *CodexProc) emit(payload []byte) {
	p.mu.Lock()
	fn := p.onOut
	p.mu.Unlock()
	if fn != nil && len(payload) > 0 {
		fn(payload)
	}
}

func (p *CodexProc) emitFailure(message string) {
	message = strings.TrimSpace(message)
	if len(message) > codexMaxErrorBytes {
		message = message[:codexMaxErrorBytes]
	}
	if message == "" {
		message = "Codex exited with an unspecified error"
	}
	payload, _ := json.Marshal(protocol.NewError("CODEX_ERROR", message))
	p.emit(payload)
	done, _ := json.Marshal(protocol.DoneMsg{Type: protocol.TypeDone})
	p.emit(done)
}

func readBoundedStderr(reader io.Reader) string {
	var out strings.Builder
	_, _ = io.Copy(&out, io.LimitReader(reader, codexMaxStderrBytes))
	_, _ = io.Copy(io.Discard, reader)
	return out.String()
}
