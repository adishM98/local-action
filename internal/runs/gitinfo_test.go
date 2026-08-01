package runs

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitInfo_RealRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "commit", "--allow-empty", "-m", "initial")

	branch, sha := gitInfo(dir)
	if branch != "main" {
		t.Errorf("branch: got %q, want main", branch)
	}
	if len(sha) < 7 {
		t.Errorf("sha: got %q, expected a short hash", sha)
	}
}

func TestGitInfo_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	branch, sha := gitInfo(dir)
	if branch != "" || sha != "" {
		t.Errorf("expected empty branch/sha for a non-git dir, got branch=%q sha=%q", branch, sha)
	}
}

func TestGitInfo_NonExistentPath(t *testing.T) {
	branch, sha := gitInfo(filepath.Join(t.TempDir(), "does-not-exist"))
	if branch != "" || sha != "" {
		t.Errorf("expected empty branch/sha for a missing path, got branch=%q sha=%q", branch, sha)
	}
}
