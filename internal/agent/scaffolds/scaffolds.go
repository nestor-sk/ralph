// Package scaffolds defines shared agent-agnostic content and formatting
// for Ralph's agent-specific scaffolding (Claude commands/rules, Cursor skills).
package scaffolds

// CommitRulesBody is the shared markdown for commit rules.
// Used by Claude (CLAUDE.md) and Cursor (ralph-commit-rules skill).
const CommitRulesBody = `
## Commit Rules

- Do NOT add Co-Authored-By headers to any commit messages.
- Commits must use only the local git user configuration.
`

// FinishSkillBody returns the shared markdown for the PRD finish skill/command.
// Used by Claude (commands/finish.md) and Cursor (ralph-finish skill).
func FinishSkillBody() string {
	return `Take the plan we have discussed and agreed upon in this conversation and structure it into a PRD JSON file.

## Output Format

Write to the PRD path specified in the system prompt. If none specified write to ` + "`.ralph/state/prd.json`" + `.

Use this exact schema:

` + "```json" + `
{
  "project": "<project name from .ralph/ralph.yaml>",
  "branchName": "ralph/<feature-name-kebab-case>",
  "description": "<one-line description of the feature>",
  "featureOverview": "<approved feature overview from the conversation>",
  "architectureOverview": "<approved architecture overview from the conversation>",
  "userStories": [
    {
      "id": "US-001",
      "title": "<short story title>",
      "description": "As a <user>, I want <feature> so that <benefit>",
      "acceptanceCriteria": [
        "Specific verifiable criterion",
        "All quality checks pass"
      ],
      "priority": 1,
      "passes": false,
      "notes": ""
    }
  ],
  "integrationTests": [
    {
      "id": "IT-001",
      "description": "<what this test verifies at a feature level>",
      "steps": [
        "Step 1: <action to perform>",
        "Step 2: <expected result to verify>"
      ],
      "passes": false,
      "failure": "",
      "notes": ""
    }
  ]
}
` + "```" + `

## Overview Fields

- Capture the approved feature overview and architecture overview from the conversation
- ` + "`featureOverview`" + `: a concise summary of the agreed-upon feature design, including the chosen approach and alternatives considered
- ` + "`architectureOverview`" + `: a concise summary of the agreed-upon architecture, including the chosen approach and alternatives considered
- If no overviews were discussed, leave these fields as empty strings

## Story Rules

- Each story must be completable in ONE context window (one Ralph iteration)
- Order by dependency: schema/data first, then backend logic, then UI
- Acceptance criteria must be specific and verifiable
- Include "All quality checks pass" in every story's acceptance criteria
- All stories start with ` + "`passes: false`" + `
- Priority determines execution order (1 = first)

## Integration Test Rules

- Include integration tests agreed upon during PRD discussion
- Each test has: id (IT-xxx), description, steps (array), passes, failure, notes
- All integration tests start with ` + "`passes: false`" + `
- ` + "`failure`" + ` field records why a test failed (empty string if not yet run)
- ` + "`notes`" + ` field captures observations or additional context

## After Writing

1. Read back the file to confirm it is valid JSON
2. Tell the user the PRD is ready and suggest: ` + "`ralph run`" + `
`
}

// ClaudeCLAUDEMD returns the content for Claude's CLAUDE.md file.
func ClaudeCLAUDEMD() string {
	return "# Ralph — Project Rules" + CommitRulesBody
}

// ClaudeFinishCommand returns the content for Claude's commands/finish.md file.
func ClaudeFinishCommand() string {
	return FinishSkillBody()
}

// CursorSkill returns a Cursor skill SKILL.md with YAML frontmatter.
// See https://cursor.com/docs/context/skills
func CursorSkill(name, description, body string, disableModelInvocation bool) string {
	front := "---\nname: " + name + "\ndescription: " + description + "\n"
	if disableModelInvocation {
		front += "disable-model-invocation: true\n"
	}
	front += "---\n\n"
	return front + body
}

// CursorCommitRulesSkill returns the content for Cursor's ralph-commit-rules skill.
func CursorCommitRulesSkill() string {
	return CursorSkill(
		"ralph-commit-rules",
		"Ralph project commit rules. Apply when making commits.",
		"# Ralph — Commit Rules"+CommitRulesBody,
		false,
	)
}

// CursorFinishSkill returns the content for Cursor's finish skill.
// Uses disable-model-invocation: true so it's invoked explicitly via /finish.
func CursorFinishSkill() string {
	return CursorSkill(
		"finish",
		"Structure the agreed plan into a PRD JSON file. Invoke when the PRD discussion is complete.",
		FinishSkillBody(),
		true,
	)
}
