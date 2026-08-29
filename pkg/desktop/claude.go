package desktop

// ResolveClaudeBinary finds a runnable Claude CLI even when the app was
// launched by Finder with a minimal PATH. Accepts wrapper commands such as
// "wrapper claude"; only the first word is resolved.
func ResolveClaudeBinary(requested string) (ResolvedCommand, error) {
	return ResolveCommand(requested, "claude", "Claude", true)
}

// ResolveCodexBinary finds Codex when Finder launches CodeAfar with a minimal
// PATH. Accepts wrapper commands such as "wrapper codex".
func ResolveCodexBinary(requested string) (ResolvedCommand, error) {
	return ResolveCommand(requested, "codex", "Codex", false)
}
