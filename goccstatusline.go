package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// Input JSON structures

type StatusInput struct {
	Model         ModelInfo     `json:"model"`
	Workspace     WorkspaceInfo `json:"workspace"`
	Cost          CostInfo      `json:"cost"`
	ContextWindow ContextWindow `json:"context_window"`
}

type ModelInfo struct {
	DisplayName string `json:"display_name"`
}

type WorkspaceInfo struct {
	CurrentDir string `json:"current_dir"`
}

type CostInfo struct {
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalDurationMS   int64   `json:"total_duration_ms"`
	TotalLinesAdded   int     `json:"total_lines_added"`
	TotalLinesRemoved int     `json:"total_lines_removed"`
}

type ContextWindow struct {
	ContextWindowSize   int           `json:"context_window_size"`
	UsedPercentage      *float64      `json:"used_percentage"`
	RemainingPercentage *float64      `json:"remaining_percentage"`
	CurrentUsage        *CurrentUsage `json:"current_usage"`
}

type CurrentUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ANSI color helpers

const (
	reset      = "\033[0m"
	bold       = "\033[1m"
	dim        = "\033[2m"
	boldCyan   = "\033[1;36m"
	boldGreen  = "\033[1;32m"
	boldBlue   = "\033[1;34m"
	boldRed    = "\033[1;31m"
	boldYellow = "\033[1;33m"
	green      = "\033[0;32m"
	red        = "\033[0;31m"
	cyan       = "\033[0;36m"
	dimWhite   = "\033[2;37m"
	yellow     = "\033[0;33m"
)

// Git types and functions

type GitInfo struct {
	RepoName    string
	Branch      string
	CommitShort string
	CommitMsg   string
	GitHubRepo  string
	Status      string
}

var githubRe = regexp.MustCompile(`github\.com[:/](.+?)(\.git)?$`)

func GetGitInfo(path string) (*GitInfo, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, err
	}

	cfg, err := repo.Config()
	if err != nil {
		return nil, err
	}

	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}

	info := &GitInfo{}

	if wt, err := repo.Worktree(); err == nil {
		info.RepoName = filepath.Base(wt.Filesystem.Root())
	}

	if head.Name().IsBranch() {
		info.Branch = head.Name().Short()
	} else {
		info.Branch = "detached"
	}

	info.CommitShort = hex.EncodeToString(headCommit.Hash[:4])[:7]

	msg := strings.SplitN(headCommit.Message, "\n", 2)[0]
	if len(msg) > 55 {
		msg = msg[:55]
	}
	info.CommitMsg = msg

	if remote, err := repo.Remote("origin"); err == nil {
		if urls := remote.Config().URLs; len(urls) > 0 {
			if m := githubRe.FindStringSubmatch(urls[0]); m != nil {
				info.GitHubRepo = m[1]
			}
		}
	}

	type statusResult struct {
		dirty bool
		err   error
	}
	statusCh := make(chan statusResult, 1)
	go func() {
		wt, err := repo.Worktree()
		if err != nil {
			statusCh <- statusResult{err: err}
			return
		}
		status, err := wt.Status()
		statusCh <- statusResult{dirty: err == nil && !status.IsClean(), err: err}
	}()

	var ahead, behind int
	if head.Name().IsBranch() {
		branchName := head.Name().Short()
		if branchCfg, ok := cfg.Branches[branchName]; ok && branchCfg.Remote != "" && branchCfg.Merge != "" {
			upstreamRef := plumbing.NewRemoteReferenceName(branchCfg.Remote, branchCfg.Merge.Short())
			if upstreamHash, err := repo.ResolveRevision(plumbing.Revision(upstreamRef)); err == nil {
				ahead, behind, _ = aheadBehind(repo, headCommit, *upstreamHash)
			}
		}
	}

	sr := <-statusCh
	statusStr := ""
	if sr.dirty {
		statusStr = "*"
	}
	if ahead > 0 {
		statusStr += fmt.Sprintf("\u2191%d", ahead)
	}
	if behind > 0 {
		statusStr += fmt.Sprintf("\u2193%d", behind)
	}
	info.Status = statusStr

	return info, nil
}

func aheadBehind(repo *git.Repository, localCommit *object.Commit, upstreamHash plumbing.Hash) (int, int, error) {
	upstreamCommit, err := repo.CommitObject(upstreamHash)
	if err != nil {
		return 0, 0, err
	}

	bases, err := localCommit.MergeBase(upstreamCommit)
	if err != nil || len(bases) == 0 {
		return 0, 0, err
	}
	baseHash := bases[0].Hash

	var (
		aheadCount int
		aheadErr   error
		wg         sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		aheadCount, aheadErr = countUntil(repo, localCommit.Hash, baseHash)
	}()

	behindCount, err := countUntil(repo, upstreamHash, baseHash)
	wg.Wait()

	if aheadErr != nil {
		return 0, 0, aheadErr
	}
	return aheadCount, behindCount, err
}

func countUntil(repo *git.Repository, from, stopAt plumbing.Hash) (int, error) {
	if from == stopAt {
		return 0, nil
	}
	iter, err := repo.Log(&git.LogOptions{From: from})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	n := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == stopAt {
			return storer.ErrStop
		}
		n++
		return nil
	})
	return n, err
}

// Output building

func buildLine1(input *StatusInput, gitInfo *GitInfo) string {
	var b strings.Builder

	// Model name (strip "Claude " prefix)
	model := input.Model.DisplayName
	if model == "" {
		model = "Claude"
	}
	model = strings.TrimPrefix(model, "Claude ")
	b.WriteString(boldCyan)
	b.WriteByte('[')
	b.WriteString(model)
	b.WriteByte(']')
	b.WriteString(reset)
	b.WriteByte(' ')

	// Repo:Branch
	if gitInfo != nil && gitInfo.RepoName != "" && gitInfo.RepoName != "no-repo" {
		b.WriteString(boldGreen)
		b.WriteString(gitInfo.RepoName)
		b.WriteString(reset)
		if gitInfo.Branch != "" {
			b.WriteByte(':')
			b.WriteString(boldBlue)
			b.WriteString(gitInfo.Branch)
			b.WriteString(reset)
		}
	}

	// Git status
	if gitInfo != nil && gitInfo.Status != "" {
		b.WriteByte(' ')
		b.WriteString(boldRed)
		b.WriteString(gitInfo.Status)
		b.WriteString(reset)
	}

	// Lines changed
	added := input.Cost.TotalLinesAdded
	removed := input.Cost.TotalLinesRemoved
	if added > 0 || removed > 0 {
		b.WriteString(" | ")
		b.WriteString(green)
		b.WriteByte('+')
		b.WriteString(strconv.Itoa(added))
		b.WriteString(reset)
		b.WriteByte('/')
		b.WriteString(red)
		b.WriteByte('-')
		b.WriteString(strconv.Itoa(removed))
		b.WriteString(reset)
	}

	return b.String()
}

func buildLine2(gitInfo *GitInfo) string {
	if gitInfo == nil || gitInfo.CommitShort == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(boldYellow)
	b.WriteByte('[')
	b.WriteString(gitInfo.CommitShort)
	b.WriteByte(']')
	b.WriteString(reset)

	if gitInfo.CommitMsg != "" {
		b.WriteByte(' ')
		b.WriteString(gitInfo.CommitMsg)
	}

	return b.String()
}

func buildLine3(input *StatusInput) string {
	var b strings.Builder

	totalTokens := input.ContextWindow.ContextWindowSize
	if totalTokens == 0 {
		totalTokens = 200000
	}

	var usedTokens, freeTokens, usagePct int

	if input.ContextWindow.UsedPercentage != nil {
		// Use official percentage (more accurate)
		usagePct = int(*input.ContextWindow.UsedPercentage)
		remainPct := 100 - usagePct
		if input.ContextWindow.RemainingPercentage != nil {
			remainPct = int(*input.ContextWindow.RemainingPercentage)
		}
		usedTokens = (totalTokens * usagePct) / 100
		freeTokens = (totalTokens * remainPct) / 100
	} else if input.ContextWindow.CurrentUsage != nil {
		// Fallback: calculate from current_usage
		cu := input.ContextWindow.CurrentUsage
		usedTokens = cu.InputTokens + cu.OutputTokens + cu.CacheCreationInputTokens + cu.CacheReadInputTokens
		freeTokens = totalTokens - usedTokens
		if totalTokens > 0 {
			usagePct = (usedTokens * 100) / totalTokens
		}
	} else {
		freeTokens = totalTokens
	}

	freeK := freeTokens / 1000

	// Brick visualization (30 bricks)
	// Pre-build single brick strings to avoid per-brick fmt calls
	const (
		usedBrick   = cyan + "\u25a0" + reset
		freeBrick   = dimWhite + "\u25a1" + reset
		totalBricks = 30
	)
	usedBricks := 0
	if totalTokens > 0 {
		usedBricks = (usedTokens * totalBricks) / totalTokens
	}
	freeBricks := totalBricks - usedBricks

	b.WriteByte('[')
	b.WriteString(strings.Repeat(usedBrick, usedBricks))
	b.WriteString(strings.Repeat(freeBrick, freeBricks))
	b.WriteByte(']')

	// Stats
	b.WriteByte(' ')
	b.WriteString(bold)
	b.WriteString(strconv.Itoa(usagePct))
	b.WriteByte('%')
	b.WriteString(reset)

	b.WriteString(" | ")
	b.WriteString(boldGreen)
	b.WriteString(strconv.Itoa(freeK))
	b.WriteString("k free")
	b.WriteString(reset)

	// Duration
	durationMS := input.Cost.TotalDurationMS
	hours := durationMS / 3600000
	mins := (durationMS % 3600000) / 60000
	b.WriteString(" | ")
	b.WriteString(strconv.FormatInt(hours, 10))
	b.WriteByte('h')
	b.WriteString(strconv.FormatInt(mins, 10))
	b.WriteByte('m')

	// Cost (only if > 0)
	if input.Cost.TotalCostUSD > 0 {
		cost := math.Round(input.Cost.TotalCostUSD*100) / 100
		b.WriteString(" | ")
		b.WriteString(yellow)
		b.WriteByte('$')
		b.WriteString(strconv.FormatFloat(cost, 'f', 2, 64))
		b.WriteString(reset)
	}

	return b.String()
}

func main() {
	var input StatusInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Determine working directory
	dir := input.Workspace.CurrentDir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	// Get git info (non-fatal if not a git repo)
	gitInfo, _ := GetGitInfo(dir)

	// Output
	fmt.Println(buildLine1(&input, gitInfo))
	if line2 := buildLine2(gitInfo); line2 != "" {
		fmt.Println(line2)
	}
	fmt.Println(buildLine3(&input))
}
