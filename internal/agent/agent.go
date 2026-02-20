package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uesteibar/ralph/internal/events"
)

// CompleteSignal is the Ralph prompt convention that agents output when done.
// Defined in loop_iteration.md; all agents receive the same instruction.
const CompleteSignal = "<promise>COMPLETE</promise>"

// InvokeOpts configures an agent CLI invocation.
type InvokeOpts struct {
	// Prompt is piped to the agent's stdin or passed as argument.
	Prompt string

	// Dir is the working directory for the agent process.
	Dir string

	// Print runs the agent in non-interactive mode and captures output.
	Print bool

	// Interactive connects stdin/stdout for a live session.
	Interactive bool

	// MaxTurns limits the number of agentic turns.
	MaxTurns int

	// Verbose enables debug logging.
	Verbose bool

	// Continue resumes the most recent conversation.
	Continue bool

	// DisallowedTools is a list of tool names the agent cannot use.
	// Use this to prevent write operations during read-only phases like refinement.
	DisallowedTools []string

	// EventHandler receives structured events during stream processing.
	// If nil, events are silently discarded.
	EventHandler events.EventHandler
}

// AgentInvoker is the interface for invoking a coding agent CLI.
type AgentInvoker interface {
	// Invoke runs the agent with the given options.
	// In Print mode it streams progress and returns the agent's output.
	// In Interactive mode it blocks until the session ends and returns empty string.
	Invoke(ctx context.Context, opts InvokeOpts) (string, error)

	// ModelName returns a human-friendly display name for the current model,
	// or empty string if unavailable.
	ModelName() string

	// ConfigDir returns the agent's config directory name (e.g. ".claude", ".cursor")
	// for workspace copy logic. Used when creating worktrees.
	ConfigDir() string
}

// UsageLimitError is returned when the agent hits a usage cap or rate limit.
// Any agent implementation can return this; the loop waits until ResetAt before retrying.
type UsageLimitError struct {
	ResetAt time.Time
	Message string
}

func (e *UsageLimitError) Error() string {
	return fmt.Sprintf("usage limit reached (resets %s): %s", e.ResetAt.Format(time.RFC3339), e.Message)
}

// ContainsComplete checks whether the agent's output contains the completion signal.
// The signal is a Ralph prompt convention; all agents receive the same instruction.
func ContainsComplete(output string) bool {
	return strings.Contains(output, CompleteSignal)
}

// Constructor creates an AgentInvoker. Used for registration.
type Constructor func() AgentInvoker

var registry = map[string]Constructor{}

// Register adds an agent constructor. Called by init() in agent implementations.
func Register(name string, fn Constructor) {
	registry[name] = fn
}

// ConfigDirFor returns the config directory name for the given agent
// (e.g. ".claude", ".cursor"). Returns ".claude" for unknown agents as fallback.
func ConfigDirFor(name string) string {
	inv, err := NewInvoker(name)
	if err != nil {
		return ".claude"
	}
	return inv.ConfigDir()
}

// NewInvoker returns an AgentInvoker for the given agent name.
// Supported: "claude", "cursor". Returns an error for unknown agents.
func NewInvoker(name string) (AgentInvoker, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q (supported: claude, cursor)", name)
	}
	return fn(), nil
}
