package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) *StatusInput {
	t.Helper()
	data, err := os.ReadFile("test/" + name + ".json")
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	var input StatusInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}
	return &input
}

// stripANSI removes ANSI escape sequences for easier assertion
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip until we hit a letter (end of escape sequence)
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
					i++
				}
				if i < len(s) {
					i++ // skip the final letter
				}
			}
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// --- JSON parsing tests ---

func TestParseFullInput(t *testing.T) {
	input := loadFixture(t, "full")

	if input.Model.DisplayName != "Claude Opus" {
		t.Errorf("model display_name = %q, want %q", input.Model.DisplayName, "Claude Opus")
	}
	if input.Workspace.CurrentDir != "/home/user/code/myproject" {
		t.Errorf("workspace current_dir = %q, want %q", input.Workspace.CurrentDir, "/home/user/code/myproject")
	}
	if input.Cost.TotalLinesAdded != 156 {
		t.Errorf("lines_added = %d, want 156", input.Cost.TotalLinesAdded)
	}
	if input.Cost.TotalLinesRemoved != 23 {
		t.Errorf("lines_removed = %d, want 23", input.Cost.TotalLinesRemoved)
	}
	if input.ContextWindow.UsedPercentage == nil || *input.ContextWindow.UsedPercentage != 45 {
		t.Error("used_percentage should be 45")
	}
	if input.ContextWindow.CurrentUsage == nil {
		t.Fatal("current_usage should not be nil")
	}
	if input.ContextWindow.CurrentUsage.CacheCreationInputTokens != 5000 {
		t.Errorf("cache_creation = %d, want 5000", input.ContextWindow.CurrentUsage.CacheCreationInputTokens)
	}
}

func TestParseMinimalInput(t *testing.T) {
	input := loadFixture(t, "minimal")

	if input.Model.DisplayName != "Claude Sonnet" {
		t.Errorf("model = %q, want %q", input.Model.DisplayName, "Claude Sonnet")
	}
	if input.Cost.TotalCostUSD != 0 {
		t.Errorf("cost should be 0, got %f", input.Cost.TotalCostUSD)
	}
	if input.ContextWindow.UsedPercentage != nil {
		t.Error("used_percentage should be nil for minimal input")
	}
	if input.ContextWindow.CurrentUsage != nil {
		t.Error("current_usage should be nil for minimal input")
	}
}

func TestParseNullPercentage(t *testing.T) {
	input := loadFixture(t, "early_session")

	if input.ContextWindow.UsedPercentage != nil {
		t.Error("used_percentage should be nil when JSON value is null")
	}
	if input.ContextWindow.RemainingPercentage != nil {
		t.Error("remaining_percentage should be nil when JSON value is null")
	}
	if input.ContextWindow.CurrentUsage != nil {
		t.Error("current_usage should be nil when JSON value is null")
	}
}

// --- Line 1 tests ---

func TestLine1ModelName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		wantModel   string
	}{
		{"strips Claude prefix", "Claude Opus", "Opus"},
		{"strips Claude prefix sonnet", "Claude Sonnet", "Sonnet"},
		{"no prefix to strip", "Opus", "Opus"},
		{"empty defaults to Claude", "", "Claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &StatusInput{Model: ModelInfo{DisplayName: tt.displayName}}
			result := stripANSI(buildLine1(input, nil))
			if !strings.HasPrefix(result, "["+tt.wantModel+"]") {
				t.Errorf("got %q, want prefix [%s]", result, tt.wantModel)
			}
		})
	}
}

func TestLine1GitInfo(t *testing.T) {
	input := &StatusInput{Model: ModelInfo{DisplayName: "Opus"}}
	git := &GitInfo{
		RepoName: "myrepo",
		Branch:   "main",
		Status:   "*\u21912",
	}
	result := stripANSI(buildLine1(input, git))

	if !strings.Contains(result, "myrepo:main") {
		t.Errorf("should contain repo:branch, got %q", result)
	}
	if !strings.Contains(result, "*\u21912") {
		t.Errorf("should contain status indicators, got %q", result)
	}
}

func TestLine1NoGit(t *testing.T) {
	input := &StatusInput{Model: ModelInfo{DisplayName: "Opus"}}
	result := stripANSI(buildLine1(input, nil))

	if strings.Contains(result, ":") {
		t.Errorf("should not contain repo:branch when no git, got %q", result)
	}
}

func TestLine1LinesChanged(t *testing.T) {
	input := &StatusInput{
		Model: ModelInfo{DisplayName: "Opus"},
		Cost:  CostInfo{TotalLinesAdded: 100, TotalLinesRemoved: 50},
	}
	result := stripANSI(buildLine1(input, nil))

	if !strings.Contains(result, "+100") || !strings.Contains(result, "-50") {
		t.Errorf("should contain +100/-50, got %q", result)
	}
}

func TestLine1NoLinesChanged(t *testing.T) {
	input := &StatusInput{Model: ModelInfo{DisplayName: "Opus"}}
	result := stripANSI(buildLine1(input, nil))

	if strings.Contains(result, "+") || strings.Contains(result, "-") {
		t.Errorf("should not show lines changed when zero, got %q", result)
	}
}

// --- Line 2 tests ---

func TestLine2WithCommit(t *testing.T) {
	git := &GitInfo{CommitShort: "abc1234", CommitMsg: "Fix the bug"}
	result := stripANSI(buildLine2(git))

	if !strings.Contains(result, "[abc1234]") {
		t.Errorf("should contain commit hash, got %q", result)
	}
	if !strings.Contains(result, "Fix the bug") {
		t.Errorf("should contain commit message, got %q", result)
	}
}

func TestLine2TruncatesLongMessage(t *testing.T) {
	git := &GitInfo{
		CommitShort: "abc1234",
		CommitMsg:   "This is a very long commit message that should be truncated at fifty five characters plus ellipsis",
	}
	result := stripANSI(buildLine2(git))

	if !strings.HasSuffix(result, "...") {
		t.Errorf("long message should end with ..., got %q", result)
	}
	// The raw content after [abc1234] should be at most 55+3 chars
	parts := strings.SplitN(result, "] ", 2)
	if len(parts) == 2 && len(parts[1]) > 58 {
		t.Errorf("truncated message too long: %d chars", len(parts[1]))
	}
}

func TestLine2NilGit(t *testing.T) {
	result := buildLine2(nil)
	if result != "" {
		t.Errorf("should return empty for nil git, got %q", result)
	}
}

// --- Line 3 tests ---

func TestLine3WithPercentage(t *testing.T) {
	input := loadFixture(t, "full")
	result := stripANSI(buildLine3(input))

	if !strings.Contains(result, "45%") {
		t.Errorf("should show 45%% usage, got %q", result)
	}
	if !strings.Contains(result, "110k free") {
		t.Errorf("should show 110k free (55%% of 200k), got %q", result)
	}
}

func TestLine3FallbackToCurrentUsage(t *testing.T) {
	input := loadFixture(t, "no_percentage_fallback")
	result := stripANSI(buildLine3(input))

	// 30000 + 10000 + 5000 = 45000 used, 155000 free = 155k
	// 45000/200000 = 22%
	if !strings.Contains(result, "22%") {
		t.Errorf("should calculate 22%% from current_usage, got %q", result)
	}
	if !strings.Contains(result, "155k free") {
		t.Errorf("should show 155k free, got %q", result)
	}
}

func TestLine3EarlySession(t *testing.T) {
	input := loadFixture(t, "early_session")
	result := stripANSI(buildLine3(input))

	if !strings.Contains(result, "0%") {
		t.Errorf("early session should show 0%%, got %q", result)
	}
	if !strings.Contains(result, "200k free") {
		t.Errorf("early session should show 200k free, got %q", result)
	}
}

func TestLine3HighContext(t *testing.T) {
	input := loadFixture(t, "high_context")
	result := stripANSI(buildLine3(input))

	if !strings.Contains(result, "92%") {
		t.Errorf("should show 92%%, got %q", result)
	}
	// 8% of 200k = 16k free
	if !strings.Contains(result, "16k free") {
		t.Errorf("should show 16k free, got %q", result)
	}
}

func TestLine3ExtendedContext(t *testing.T) {
	input := loadFixture(t, "extended_context")
	result := stripANSI(buildLine3(input))

	// 70% of 1000000 = 700k free
	if !strings.Contains(result, "700k free") {
		t.Errorf("should show 700k free for 1M context, got %q", result)
	}
	if !strings.Contains(result, "30%") {
		t.Errorf("should show 30%%, got %q", result)
	}
}

func TestLine3Duration(t *testing.T) {
	input := loadFixture(t, "full")
	result := stripANSI(buildLine3(input))

	// 3661000ms = 1h 1m
	if !strings.Contains(result, "1h1m") {
		t.Errorf("should show 1h1m duration, got %q", result)
	}
}

func TestLine3DurationLong(t *testing.T) {
	input := loadFixture(t, "extended_context")
	result := stripANSI(buildLine3(input))

	// 18000000ms = 5h 0m
	if !strings.Contains(result, "5h0m") {
		t.Errorf("should show 5h0m duration, got %q", result)
	}
}

func TestLine3CostShown(t *testing.T) {
	input := loadFixture(t, "full")
	result := stripANSI(buildLine3(input))

	if !strings.Contains(result, "$1.23") {
		t.Errorf("should show $1.23 (rounded), got %q", result)
	}
}

func TestLine3CostHiddenWhenZero(t *testing.T) {
	input := loadFixture(t, "zero_cost")
	result := stripANSI(buildLine3(input))

	if strings.Contains(result, "$") {
		t.Errorf("should not show cost when zero, got %q", result)
	}
}

func TestLine3DefaultContextSize(t *testing.T) {
	input := loadFixture(t, "minimal")
	result := stripANSI(buildLine3(input))

	// No context_window_size set, should default to 200000
	if !strings.Contains(result, "200k free") {
		t.Errorf("should default to 200k context, got %q", result)
	}
}

func TestLine3BrickCount(t *testing.T) {
	input := loadFixture(t, "full")
	result := buildLine3(input)

	// Count filled bricks (■) and empty bricks (□)
	filled := strings.Count(result, "\u25a0")
	empty := strings.Count(result, "\u25a1")

	if filled+empty != 30 {
		t.Errorf("total bricks should be 30, got %d filled + %d empty = %d", filled, empty, filled+empty)
	}
	// 45% of 30 = 13 filled
	if filled != 13 {
		t.Errorf("expected 13 filled bricks for 45%%, got %d", filled)
	}
}

func TestLine3BrickCountHighUsage(t *testing.T) {
	input := loadFixture(t, "high_context")
	result := buildLine3(input)

	filled := strings.Count(result, "\u25a0")
	empty := strings.Count(result, "\u25a1")

	if filled+empty != 30 {
		t.Errorf("total bricks should be 30, got %d", filled+empty)
	}
	// 92% of 30 = 27 filled
	if filled != 27 {
		t.Errorf("expected 27 filled bricks for 92%%, got %d", filled)
	}
}

// --- ANSI output tests ---

func TestLine1ContainsANSI(t *testing.T) {
	input := &StatusInput{Model: ModelInfo{DisplayName: "Opus"}}
	result := buildLine1(input, nil)

	if !strings.Contains(result, "\033[") {
		t.Error("line1 should contain ANSI escape codes")
	}
	if !strings.Contains(result, boldCyan) {
		t.Error("model should be bold cyan")
	}
}

func TestLine3ContainsANSI(t *testing.T) {
	input := loadFixture(t, "full")
	result := buildLine3(input)

	if !strings.Contains(result, cyan) {
		t.Error("used bricks should use cyan color")
	}
	if !strings.Contains(result, dimWhite) {
		t.Error("free bricks should use dim white color")
	}
	if !strings.Contains(result, boldGreen) {
		t.Error("free tokens should use bold green")
	}
}
