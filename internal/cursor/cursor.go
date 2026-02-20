package cursor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uesteibar/ralph/internal/agent"
	"github.com/uesteibar/ralph/internal/events"
	"github.com/uesteibar/ralph/internal/shell"
)

func init() {
	agent.Register("cursor", func() agent.AgentInvoker { return &Agent{} })
}

// Agent implements agent.AgentInvoker for the Cursor CLI.
type Agent struct{}

func (a *Agent) Invoke(ctx context.Context, opts agent.InvokeOpts) (string, error) {
	r := &shell.Runner{Dir: opts.Dir}

	if opts.Interactive {
		args := buildArgs(opts)
		return "", r.RunInteractive(ctx, "agent", args...)
	}

	return runWithStreamJSON(ctx, opts)
}

func (a *Agent) ModelName() string {
	return modelName()
}

func (a *Agent) ConfigDir() string {
	return ".cursor"
}

// cursorStreamEvent represents a JSON event from Cursor CLI stream-json output.
// Cursor uses type: user, assistant, tool_call, result.
type cursorStreamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	// For result events
	DurationMS int `json:"duration_ms,omitempty"`
	// For assistant events
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content,omitempty"`
	} `json:"message,omitempty"`
	// For tool_call events (subtype "started")
	ToolCall struct {
		ShellToolCall   *struct{ Args struct{ Command string `json:"command"` } } `json:"shellToolCall,omitempty"`
		ReadToolCall    *struct{ Args struct{ Path string `json:"path"` } } `json:"readToolCall,omitempty"`
		EditToolCall    *struct{ Args struct{ Path string `json:"path"` } } `json:"editToolCall,omitempty"`
		WriteToolCall   *struct{ Args struct{ Path string `json:"path"`; FileText string `json:"fileText"` } } `json:"writeToolCall,omitempty"`
		GrepToolCall    *struct{ Args struct{ Pattern string `json:"pattern"`; Path string `json:"path"` } } `json:"grepToolCall,omitempty"`
		GlobToolCall    *struct{ Args struct{ GlobPattern string `json:"globPattern"`; TargetDirectory string `json:"targetDirectory"` } } `json:"globToolCall,omitempty"`
		LSToolCall      *struct{ Args struct{ Path string `json:"path"` } } `json:"lsToolCall,omitempty"`
		TodoToolCall    *struct{ Args struct{ Description string `json:"description"` } } `json:"todoToolCall,omitempty"`
		DeleteToolCall  *struct{ Args struct{ Path string `json:"path"` } } `json:"deleteToolCall,omitempty"`
	} `json:"tool_call,omitempty"`
}

func runWithStreamJSON(ctx context.Context, opts agent.InvokeOpts) (string, error) {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"-f",
	}

	if opts.Verbose {
		args = append(args, "--verbose")
	}

	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}

	// Cursor accepts prompt as positional argument
	args = append(args, opts.Prompt)

	workDir := opts.Dir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	cmd := exec.CommandContext(ctx, "agent", args...)
	cmd.Dir = workDir

	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting agent: %w", err)
	}

	var result string
	var assistantText strings.Builder
	var nonJSONLines []string
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var numTurns int
	var durationMS int

	for scanner.Scan() {
		line := scanner.Text()
		var ev cursorStreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				nonJSONLines = append(nonJSONLines, trimmed)
			}
			continue
		}

		switch ev.Type {
		case "assistant":
			for _, content := range ev.Message.Content {
				if content.Type == "text" && content.Text != "" {
					assistantText.WriteString(content.Text)
					assistantText.WriteByte('\n')
					emitEvent(opts.EventHandler, events.AgentText{Text: content.Text})
				}
			}
		case "tool_call":
			if ev.Subtype == "started" {
				name, detail := toolCallDetail(ev, workDir)
				if name != "" {
					emitEvent(opts.EventHandler, events.ToolUse{Name: name, Detail: detail})
				}
			}
		case "result":
			durationMS = ev.DurationMS
			numTurns++
		}
	}

	// Capture result from last assistant text if no explicit result
	result = assistantText.String()

	if numTurns > 0 {
		emitEvent(opts.EventHandler, events.InvocationDone{
			NumTurns:   numTurns,
			DurationMS: durationMS,
		})
	}

	waitErr := cmd.Wait()

	allOutput := result + "\n" + assistantText.String() + "\n" + stderrBuf.String() + "\n" + strings.Join(nonJSONLines, "\n")
	if ulErr := parseUsageLimit(allOutput); ulErr != nil {
		return result, ulErr
	}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return result, &shell.ExitError{
				Code:   exitErr.ExitCode(),
				Stderr: strings.TrimSpace(stderrBuf.String()),
				Cmd:    "agent",
			}
		}
		return result, fmt.Errorf("running agent: %w", waitErr)
	}

	return result, nil
}

func toolCallDetail(ev cursorStreamEvent, workDir string) (name, detail string) {
	switch {
	case ev.ToolCall.ShellToolCall != nil:
		cmd := ev.ToolCall.ShellToolCall.Args.Command
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		return "Bash", cmd
	case ev.ToolCall.ReadToolCall != nil:
		return "Read", relativePath(ev.ToolCall.ReadToolCall.Args.Path, workDir)
	case ev.ToolCall.EditToolCall != nil:
		return "Edit", relativePath(ev.ToolCall.EditToolCall.Args.Path, workDir)
	case ev.ToolCall.WriteToolCall != nil:
		return "Write", relativePath(ev.ToolCall.WriteToolCall.Args.Path, workDir)
	case ev.ToolCall.GrepToolCall != nil:
		a := ev.ToolCall.GrepToolCall.Args
		d := fmt.Sprintf("%q", a.Pattern)
		if a.Path != "" {
			d += " in " + relativePath(a.Path, workDir)
		}
		return "Grep", d
	case ev.ToolCall.GlobToolCall != nil:
		return "Glob", ev.ToolCall.GlobToolCall.Args.GlobPattern
	case ev.ToolCall.LSToolCall != nil:
		return "LS", relativePath(ev.ToolCall.LSToolCall.Args.Path, workDir)
	case ev.ToolCall.TodoToolCall != nil:
		return "Task", ev.ToolCall.TodoToolCall.Args.Description
	case ev.ToolCall.DeleteToolCall != nil:
		return "Delete", relativePath(ev.ToolCall.DeleteToolCall.Args.Path, workDir)
	default:
		return "", ""
	}
}

func relativePath(path, workDir string) string {
	if workDir == "" {
		return path
	}
	rel, err := filepath.Rel(workDir, path)
	if err != nil {
		return path
	}
	return rel
}

func emitEvent(h events.EventHandler, e events.Event) {
	if h != nil {
		h.Handle(e)
	}
}

func parseUsageLimit(output string) *agent.UsageLimitError {
	if !isUsageLimitError(output) {
		return nil
	}
	// Cursor may use similar messaging; use fallback reset time
	resetAt := time.Now().Add(30 * time.Minute)
	return &agent.UsageLimitError{
		ResetAt: resetAt,
		Message: "usage limit reached",
	}
}

func isUsageLimitError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "usage limit") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "quota exceeded")
}

func modelName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "agent", "config", "get", "model")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func buildArgs(opts agent.InvokeOpts) []string {
	var args []string
	args = append(args, "-f")
	if opts.Print {
		args = append(args, "-p")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}
	if opts.Continue {
		args = append(args, "--resume")
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}
	if opts.Prompt != "" && !opts.Print {
		args = append(args, "--system-prompt", opts.Prompt)
	}
	return args
}
