package scaffolds

import (
	"strings"
	"testing"
)

func TestCommitRulesBody_ContainsExpectedContent(t *testing.T) {
	body := CommitRulesBody
	if !strings.Contains(body, "Co-Authored-By") {
		t.Error("CommitRulesBody should contain Co-Authored-By")
	}
	if !strings.Contains(body, "local git user configuration") {
		t.Error("CommitRulesBody should contain local git user configuration")
	}
}

func TestFinishSkillBody_ContainsExpectedContent(t *testing.T) {
	body := FinishSkillBody()
	if !strings.Contains(body, "featureOverview") {
		t.Error("FinishSkillBody should contain featureOverview")
	}
	if !strings.Contains(body, "architectureOverview") {
		t.Error("FinishSkillBody should contain architectureOverview")
	}
	if !strings.Contains(body, ".ralph/state/prd.json") {
		t.Error("FinishSkillBody should contain PRD path")
	}
}

func TestClaudeCLAUDEMD_ContainsCommitRules(t *testing.T) {
	content := ClaudeCLAUDEMD()
	if !strings.Contains(content, "# Ralph — Project Rules") {
		t.Error("Claude CLAUDE.md should have title")
	}
	if !strings.Contains(content, CommitRulesBody) {
		t.Error("Claude CLAUDE.md should include CommitRulesBody")
	}
}

func TestClaudeFinishCommand_MatchesFinishSkillBody(t *testing.T) {
	cmd := ClaudeFinishCommand()
	body := FinishSkillBody()
	if cmd != body {
		t.Error("Claude finish command should match FinishSkillBody")
	}
}

func TestCursorCommitRulesSkill_HasValidFrontmatter(t *testing.T) {
	content := CursorCommitRulesSkill()
	if !strings.Contains(content, "---") {
		t.Error("Cursor skill should have YAML frontmatter")
	}
	if !strings.Contains(content, "name: ralph-commit-rules") {
		t.Error("Cursor skill should have name in frontmatter")
	}
	if !strings.Contains(content, "description:") {
		t.Error("Cursor skill should have description in frontmatter")
	}
	// Should not have disable-model-invocation (commit rules apply when relevant)
	if strings.Contains(content, "disable-model-invocation") {
		t.Error("Commit rules skill should not have disable-model-invocation")
	}
	if !strings.Contains(content, CommitRulesBody) {
		t.Error("Cursor commit rules skill should include CommitRulesBody")
	}
}

func TestCursorFinishSkill_HasValidFrontmatter(t *testing.T) {
	content := CursorFinishSkill()
	if !strings.Contains(content, "name: finish") {
		t.Error("Cursor finish skill should have name in frontmatter")
	}
	if !strings.Contains(content, "disable-model-invocation: true") {
		t.Error("Cursor finish skill should have disable-model-invocation: true")
	}
	if !strings.Contains(content, FinishSkillBody()) {
		t.Error("Cursor finish skill should include FinishSkillBody")
	}
}
