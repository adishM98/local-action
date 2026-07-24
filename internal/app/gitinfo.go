package app

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// gitInfo returns the current branch and short commit SHA checked out at
// repoPath, or ("", "") if repoPath isn't a git repo, git isn't installed,
// or the lookup times out — never fails the run over this being unset.
func gitInfo(repoPath string) (branch, sha string) {
	branch = gitOutput(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	sha = gitOutput(repoPath, "rev-parse", "--short", "HEAD")
	return branch, sha
}

func gitOutput(repoPath string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
