package commands

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	_ "github.com/uesteibar/ralph/internal/claude" // register claude agent
	_ "github.com/uesteibar/ralph/internal/cursor" // register cursor agent
	"github.com/uesteibar/ralph/internal/agent"
	"github.com/uesteibar/ralph/internal/agent/scaffolds"
	"github.com/uesteibar/ralph/internal/config"
	"github.com/uesteibar/ralph/internal/knowledge"
)

// invokeAgentFn is the function used to invoke the agent CLI. It can be
// overridden in tests to avoid calling the real CLI.
var invokeAgentFn = func(ctx context.Context, inv agent.AgentInvoker, opts agent.InvokeOpts) (string, error) {
	return inv.Invoke(ctx, opts)
}

const qualityCheckPrompt = `Analyze this codebase and detect the quality check commands that should be run (tests, linting, type checking, formatting, etc.).

Output ONLY a YAML list of shell commands, one per line. No explanation, no code fences, no surrounding text. Example format:
- "npm test"
- "npm run lint"
`

const defaultConfigTemplate = `project: %s

repo:
  default_base: main
  branch_pattern: "^ralph/[a-zA-Z0-9._-]+$"

paths:
  tasks_dir: ".ralph/tasks"
  skills_dir: ".ralph/skills"

# agent: claude   # or "cursor" — default is claude
quality_checks:
  - "npm test"
  # - "npm run lint"
`

// Init scaffolds the .ralph/ directory in the current project.
// It is idempotent: re-running it skips existing files and ensures
// all directories and skills are up to date.
// The in parameter provides stdin for interactive prompts.
func Init(args []string, in io.Reader) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	scanner := bufio.NewScanner(in)

	ralphDir := filepath.Join(cwd, ".ralph")
	stateDir := filepath.Join(ralphDir, "state")
	configPath := filepath.Join(ralphDir, "ralph.yaml")

	// Resolve agent for scaffolding (env, existing config, or default)
	inv, invErr := resolveAgentForDir(cwd)
	agentName := "claude"
	if invErr == nil {
		agentName = inv.ConfigDir()
		if agentName == ".claude" {
			agentName = "claude"
		} else if agentName == ".cursor" {
			agentName = "cursor"
		}
	}

	var created, skipped []string

	// --- Git tracking prompt ---
	gitTrackChoice := promptGitTracking(scanner)

	// --- LLM analysis prompt ---
	useLLM := promptLLMAnalysis(scanner)

	// Create directory structure (MkdirAll is idempotent)
	dirs := []string{
		ralphDir,
		filepath.Join(ralphDir, "tasks"),
		filepath.Join(ralphDir, "skills"),
		filepath.Join(ralphDir, "knowledge"),
		filepath.Join(ralphDir, "workspaces"),
		stateDir,
		filepath.Join(stateDir, "archive"),
	}
	if agentName == "claude" {
		dirs = append(dirs, filepath.Join(cwd, ".claude", "commands"))
	} else if agentName == "cursor" {
		dirs = append(dirs, filepath.Join(cwd, ".cursor", "skills", "ralph-commit-rules"))
		dirs = append(dirs, filepath.Join(cwd, ".cursor", "skills", "finish"))
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Initialize workspaces.json (only if it doesn't exist)
	workspacesJSONPath := filepath.Join(stateDir, "workspaces.json")
	if _, err := os.Stat(workspacesJSONPath); err != nil {
		if err := os.WriteFile(workspacesJSONPath, []byte("[]"), 0644); err != nil {
			return fmt.Errorf("writing workspaces.json: %w", err)
		}
		created = append(created, ".ralph/state/workspaces.json")
	}

	// Write config (only if it doesn't exist)
	if _, err := os.Stat(configPath); err != nil {
		projectName := filepath.Base(cwd)

		var qualityChecks []string
		if useLLM {
			detected, llmErr := detectQualityChecks(cwd)
			if llmErr != nil {
				fmt.Printf("Warning: agent analysis failed (%v), using default template.\n", llmErr)
			} else {
				qualityChecks = detected
			}
		}

		var content string
		if len(qualityChecks) > 0 {
			content = buildConfigWithChecks(projectName, qualityChecks)
		} else {
			content = fmt.Sprintf(defaultConfigTemplate, projectName)
		}

		var cfg config.Config
		if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
			return fmt.Errorf("generated config is invalid (bug): %w", err)
		}

		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
		created = append(created, ".ralph/ralph.yaml")
	} else {
		skipped = append(skipped, ".ralph/ralph.yaml (already exists)")
	}

	// Write .gitkeep files (idempotent)
	for _, dir := range []string{"tasks", "skills"} {
		keepPath := filepath.Join(ralphDir, dir, ".gitkeep")
		if err := os.WriteFile(keepPath, []byte(""), 0644); err != nil {
			return fmt.Errorf("writing .gitkeep: %w", err)
		}
	}

	// Seed knowledge base README (only if it doesn't exist)
	if err := knowledge.SeedReadme(cwd); err != nil {
		return fmt.Errorf("seeding knowledge base: %w", err)
	}

	// Install agent-specific scaffolding (from shared scaffolds)
	if agentName == "claude" {
		finishSkillPath := filepath.Join(cwd, ".claude", "commands", "finish.md")
		if err := os.WriteFile(finishSkillPath, []byte(scaffolds.ClaudeFinishCommand()), 0644); err != nil {
			return fmt.Errorf("writing finish skill: %w", err)
		}
		created = append(created, ".claude/commands/finish.md")

		claudeMDPath := filepath.Join(cwd, ".claude", "CLAUDE.md")
		if err := os.WriteFile(claudeMDPath, []byte(scaffolds.ClaudeCLAUDEMD()), 0644); err != nil {
			return fmt.Errorf("writing CLAUDE.md: %w", err)
		}
		created = append(created, ".claude/CLAUDE.md")
	} else if agentName == "cursor" {
		commitRulesPath := filepath.Join(cwd, ".cursor", "skills", "ralph-commit-rules", "SKILL.md")
		if err := os.WriteFile(commitRulesPath, []byte(scaffolds.CursorCommitRulesSkill()), 0644); err != nil {
			return fmt.Errorf("writing Cursor ralph-commit-rules skill: %w", err)
		}
		created = append(created, ".cursor/skills/ralph-commit-rules/SKILL.md")

		finishSkillPath := filepath.Join(cwd, ".cursor", "skills", "finish", "SKILL.md")
		if err := os.WriteFile(finishSkillPath, []byte(scaffolds.CursorFinishSkill()), 0644); err != nil {
			return fmt.Errorf("writing Cursor finish skill: %w", err)
		}
		created = append(created, ".cursor/skills/finish/SKILL.md")
	}

	// Ensure appropriate paths are in .gitignore based on user's choice
	ensureGitignoreEntries(cwd, gitTrackChoice)

	fmt.Fprintf(os.Stderr, "initialized .ralph/ in %s\n", cwd)
	fmt.Println("Ralph initialized.")
	if len(created) > 0 {
		fmt.Println()
		fmt.Println("Created:")
		for _, c := range created {
			fmt.Printf("  %s\n", c)
		}
	}
	if len(skipped) > 0 {
		fmt.Println()
		fmt.Println("Skipped:")
		for _, s := range skipped {
			fmt.Printf("  %s\n", s)
		}
	}
	fmt.Println()
	fmt.Println("Shell integration (add to your ~/.bashrc or ~/.zshrc):")
	fmt.Println()
	fmt.Println("  eval \"$(ralph shell-init)\"")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .ralph/ralph.yaml (set quality checks, project details)")
	fmt.Println("  2. ralph new <name>     (creates workspace and starts PRD creation)")
	fmt.Println("  3. ralph run            (execute the autonomous loop)")

	return nil
}

// promptGitTracking asks the user how to handle .ralph/ in git.
// Returns 1 for "track in git" or 2 for "keep local".
func promptGitTracking(scanner *bufio.Scanner) int {
	fmt.Println("How should .ralph/ be handled in git?")
	fmt.Println()
	fmt.Println("  1) Track in git — share config with your team")
	fmt.Println("  2) Keep local  — gitignore the entire .ralph/ directory")
	fmt.Println()

	for {
		fmt.Print("Choose [1/2]: ")
		if !scanner.Scan() {
			return 1
		}
		choice := trimSpace(scanner.Text())
		if choice == "1" {
			return 1
		}
		if choice == "2" {
			return 2
		}
		fmt.Println("Please enter 1 or 2.")
	}
}

// promptLLMAnalysis asks whether to use Claude for quality check detection.
// Returns true if the user accepts (default yes).
func promptLLMAnalysis(scanner *bufio.Scanner) bool {
	fmt.Print("Use Claude to detect quality checks? [Y/n] ")
	if !scanner.Scan() {
		return true
	}
	answer := trimSpace(scanner.Text())
	if answer == "" || answer == "Y" || answer == "y" || answer == "yes" || answer == "Yes" {
		return true
	}
	return false
}

func ensureGitignoreEntries(dir string, gitTrackChoice int) {
	gitignorePath := filepath.Join(dir, ".gitignore")
	content, _ := os.ReadFile(gitignorePath)

	var entries []string
	if gitTrackChoice == 2 {
		entries = []string{".ralph/"}
	} else {
		entries = []string{".ralph/workspaces/", ".ralph/state/"}
	}

	var toAdd []string
	existing := string(content)
	for _, entry := range entries {
		if !containsLine(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	// Add a newline separator if file doesn't end with one
	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}

	f.WriteString("\n# Ralph\n")
	for _, entry := range toAdd {
		f.WriteString(entry + "\n")
	}
}

func containsLine(s, line string) bool {
	for _, l := range splitLines(s) {
		if trimSpace(l) == line {
			return true
		}
	}
	return false
}

// String helpers to avoid importing strings (keeping deps minimal within commands).
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// detectQualityChecks invokes the agent CLI to analyze the codebase and return
// a list of quality check commands.
func detectQualityChecks(dir string) ([]string, error) {
	fmt.Println("Analyzing the codebase...")

	inv, err := resolveAgentForDir(dir)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	output, err := invokeAgentFn(ctx, inv, agent.InvokeOpts{
		Prompt:   qualityCheckPrompt,
		Dir:      dir,
		Print:    true,
		MaxTurns: 3,
	})
	if err != nil {
		return nil, err
	}
	return parseQualityChecks(output)
}

// resolveAgentForDir returns the agent invoker for the given directory.
// Uses RALPH_AGENT env, config.Discover, or "claude" as fallback.
func resolveAgentForDir(dir string) (agent.AgentInvoker, error) {
	if a := os.Getenv("RALPH_AGENT"); a != "" {
		return agent.NewInvoker(a)
	}
	if cfg, err := config.Discover(dir); err == nil {
		return agent.NewInvoker(cfg.Agent)
	}
	return agent.NewInvoker("claude")
}

// parseQualityChecks parses Claude's YAML list output into a string slice.
func parseQualityChecks(output string) ([]string, error) {
	var checks []string
	if err := yaml.Unmarshal([]byte(output), &checks); err != nil {
		return nil, fmt.Errorf("parsing Claude output as YAML list: %w", err)
	}
	if len(checks) == 0 {
		return nil, fmt.Errorf("Claude returned no quality checks")
	}
	return checks, nil
}

// buildConfigWithChecks generates ralph.yaml content with the given quality checks.
func buildConfigWithChecks(projectName string, checks []string) string {
	cfg := config.Config{
		Project: projectName,
		Repo: config.RepoConfig{
			DefaultBase:   "main",
			BranchPattern: "^ralph/[a-zA-Z0-9._-]+$",
		},
		Paths: config.PathsConfig{
			TasksDir:  ".ralph/tasks",
			SkillsDir: ".ralph/skills",
		},
		QualityChecks: checks,
	}
	out, _ := yaml.Marshal(&cfg)
	return string(out)
}
