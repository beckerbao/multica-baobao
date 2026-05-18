package daemon

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxChangedFiles            = 200
	maxChangeSummaryPayloadLen = 256 * 1024
	gitCollectTimeout          = 3 * time.Second
)

var shortStatRe = regexp.MustCompile(`(\d+)\s+files?\s+changed(?:,\s+(\d+)\s+insertions?\(\+\))?(?:,\s+(\d+)\s+deletions?\(-\))?`)

func collectTaskChangeSummary(workDir string) *TaskChangeSummary {
	summary := &TaskChangeSummary{CollectStatus: "error"}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return summary
	}
	workDir = filepath.Clean(workDir)

	// Is git worktree?
	if out, err := runGitCommand(workDir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		summary.CollectStatus = "git_unavailable"
		return summary
	}

	headBefore, _ := runGitCommand(workDir, "rev-parse", "HEAD")
	branch, _ := runGitCommand(workDir, "rev-parse", "--abbrev-ref", "HEAD")
	nameStatusRaw, err := runGitCommand(workDir, "diff", "--name-status")
	if err != nil {
		summary.CollectStatus = "error"
		return summary
	}
	statusRaw, err := runGitCommand(workDir, "status", "--porcelain")
	if err != nil {
		summary.CollectStatus = "error"
		return summary
	}
	shortStatWorktreeRaw, _ := runGitCommand(workDir, "diff", "--shortstat")
	shortStatCachedRaw, _ := runGitCommand(workDir, "diff", "--cached", "--shortstat")
	headAfter, _ := runGitCommand(workDir, "rev-parse", "HEAD")

	summary.GitBranch = strings.TrimSpace(branch)
	summary.HeadBefore = strings.TrimSpace(headBefore)
	summary.HeadAfter = strings.TrimSpace(headAfter)
	diffFiles := parseNameStatus(nameStatusRaw)
	statusFiles := parsePorcelainStatus(statusRaw)
	summary.ChangedFiles = mergeChangeFiles(diffFiles, statusFiles)
	worktreeStat := parseShortStat(shortStatWorktreeRaw)
	cachedStat := parseShortStat(shortStatCachedRaw)
	summary.DiffStat = TaskDiffStat{
		FilesChanged: worktreeStat.FilesChanged + cachedStat.FilesChanged,
		Insertions:   worktreeStat.Insertions + cachedStat.Insertions,
		Deletions:    worktreeStat.Deletions + cachedStat.Deletions,
	}
	summary.CollectStatus = "ok"

	if len(summary.ChangedFiles) > maxChangedFiles {
		summary.ChangedFiles = summary.ChangedFiles[:maxChangedFiles]
		summary.CollectStatus = "truncated"
	}
	if estimateSummarySize(summary) > maxChangeSummaryPayloadLen {
		// Trim files until payload estimate fits.
		for len(summary.ChangedFiles) > 0 && estimateSummarySize(summary) > maxChangeSummaryPayloadLen {
			summary.ChangedFiles = summary.ChangedFiles[:len(summary.ChangedFiles)-1]
		}
		summary.CollectStatus = "truncated"
	}
	return summary
}

func runGitCommand(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCollectTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func parseNameStatus(raw string) []TaskChangeFile {
	if strings.TrimSpace(raw) == "" {
		return []TaskChangeFile{}
	}
	lines := strings.Split(raw, "\n")
	files := make([]TaskChangeFile, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		if path == "" || strings.Contains(path, "..") {
			continue
		}
		files = append(files, TaskChangeFile{Path: path, Status: status})
	}
	return files
}

func parsePorcelainStatus(raw string) []TaskChangeFile {
	if strings.TrimSpace(raw) == "" {
		return []TaskChangeFile{}
	}
	lines := strings.Split(raw, "\n")
	files := make([]TaskChangeFile, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if path == "" || strings.Contains(path, "..") {
			continue
		}
		files = append(files, TaskChangeFile{Path: path, Status: status})
	}
	return files
}

func mergeChangeFiles(primary, secondary []TaskChangeFile) []TaskChangeFile {
	if len(primary) == 0 {
		return secondary
	}
	if len(secondary) == 0 {
		return primary
	}
	seen := make(map[string]struct{}, len(primary))
	out := make([]TaskChangeFile, 0, len(primary)+len(secondary))
	for _, item := range primary {
		out = append(out, item)
		seen[item.Path] = struct{}{}
	}
	for _, item := range secondary {
		if _, ok := seen[item.Path]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func parseShortStat(raw string) TaskDiffStat {
	out := TaskDiffStat{}
	m := shortStatRe.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) == 0 {
		return out
	}
	out.FilesChanged, _ = strconv.Atoi(m[1])
	if len(m) > 2 && m[2] != "" {
		out.Insertions, _ = strconv.Atoi(m[2])
	}
	if len(m) > 3 && m[3] != "" {
		out.Deletions, _ = strconv.Atoi(m[3])
	}
	return out
}

func estimateSummarySize(summary *TaskChangeSummary) int {
	if summary == nil {
		return 0
	}
	size := len(summary.CollectStatus) + len(summary.GitBranch) + len(summary.HeadBefore) + len(summary.HeadAfter) + 64
	size += 32 // diff stat
	for _, f := range summary.ChangedFiles {
		size += len(f.Path) + len(f.Status) + 16
	}
	return size
}
