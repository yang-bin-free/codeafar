package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
)

// ClaudeConfig 是启动 claude 子进程所需的参数。
type ClaudeConfig struct {
	Bin          string   // claude 可执行文件路径（默认 "claude"）
	BinArgs      []string // 包装命令的前置参数（如 "wrapper claude" 的 "claude"）
	Cwd          string   // 工作目录
	SessionID    string   // 固定 session-id，支持 --resume
	Permission   string   // bypassPermissions | acceptEdits | default
	AddDirs      []string // 额外 --add-dir
	Resume       bool
	AllowedTools []string
}

// OutputFunc 接收一行 claude stdout 的原始 JSON。
type OutputFunc func(payload []byte)

// ClaudeProc 驱动单个 claude 子进程（translate 层）。
type ClaudeProc struct {
	cfg    ClaudeConfig
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	mu    sync.Mutex
	onOut OutputFunc
}

// NewClaudeProc 按配置创建 claude 子进程驱动（未启动）。
func NewClaudeProc(cfg ClaudeConfig) *ClaudeProc {
	if cfg.Bin == "" {
		cfg.Bin = "claude"
	}
	return &ClaudeProc{cfg: cfg}
}

// OnOutput 注册 stdout 行回调。
func (p *ClaudeProc) OnOutput(fn OutputFunc) {
	p.mu.Lock()
	p.onOut = fn
	p.mu.Unlock()
}

// buildArgs 组装 claude CLI 参数（README §4.5）。
func (p *ClaudeProc) buildArgs() []string {
	args := []string{"--print"}
	if p.cfg.Resume {
		args = append(args, "--resume", p.cfg.SessionID)
	} else {
		args = append(args, "--session-id", p.cfg.SessionID)
	}
	args = append(args,
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--permission-mode", p.cfg.Permission,
		"--replay-user-messages",
	)
	if len(p.cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, p.cfg.AllowedTools...)
	}
	for _, d := range p.cfg.AddDirs {
		args = append(args, "--add-dir", d)
	}
	return args
}

// Start 启动子进程并开始读取 stdout。
func (p *ClaudeProc) Start() error {
	cmd := exec.Command(p.cfg.Bin, append(append([]string(nil), p.cfg.BinArgs...), p.buildArgs()...)...)
	cmd.Dir = p.cfg.Cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	p.cmd, p.stdin, p.stdout = cmd, stdin, stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	go p.readLoop()
	return nil
}

func (p *ClaudeProc) readLoop() {
	sc := bufio.NewScanner(p.stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		p.mu.Lock()
		fn := p.onOut
		p.mu.Unlock()
		if fn != nil && len(line) > 0 {
			fn(line)
		}
	}
}

// Send 把用户文本封成 stream-json 一行写入 stdin。
func (p *ClaudeProc) Send(text string) error {
	if p.stdin == nil {
		return errors.New("claude proc not started")
	}
	msg := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": text,
		},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = p.stdin.Write(b)
	return err
}

// Stop 关闭 stdin 并等待进程退出。
func (p *ClaudeProc) Stop() error {
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Wait()
	}
	return nil
}
