package runs

import (
	"reflect"
	"testing"
)

func TestBuildArgv_NoInputs(t *testing.T) {
	req := RunRequest{WorkflowFile: ".github/workflows/ci.yml", Event: "push"}
	got := BuildArgv(req, "/tmp/secrets.env", "/tmp/vars.env", "/tmp/empty.env", "")
	want := []string{
		"push", "-W", ".github/workflows/ci.yml", "--json",
		"--secret-file", "/tmp/secrets.env", "--var-file", "/tmp/vars.env", "--env-file", "/tmp/empty.env",
		"--container-architecture", "linux/amd64",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildArgv_EventPayloadOmittedWhenEmpty(t *testing.T) {
	req := RunRequest{WorkflowFile: "ci.yml", Event: "push"}
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env", "/tmp/e.env", "")
	for _, arg := range got {
		if arg == "-e" {
			t.Fatalf("expected no -e flag when eventPayloadFile is empty, got %v", got)
		}
	}
}

func TestBuildArgv_EventPayloadIncludedWhenSet(t *testing.T) {
	req := RunRequest{WorkflowFile: "ci.yml", Event: "workflow_dispatch"}
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env", "/tmp/e.env", "/tmp/event.json")
	want := []string{
		"workflow_dispatch", "-W", "ci.yml", "--json",
		"--secret-file", "/tmp/s.env", "--var-file", "/tmp/v.env", "--env-file", "/tmp/e.env",
		"--container-architecture", "linux/amd64",
		"-e", "/tmp/event.json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildArgv_InputsAreSortedForDeterminism(t *testing.T) {
	req := RunRequest{
		WorkflowFile: "deploy.yml",
		Event:        "workflow_dispatch",
		Inputs:       map[string]string{"zeta": "1", "alpha": "hello world"},
	}
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env", "/tmp/empty.env", "")
	want := []string{
		"workflow_dispatch", "-W", "deploy.yml", "--json",
		"--secret-file", "/tmp/s.env", "--var-file", "/tmp/v.env", "--env-file", "/tmp/empty.env",
		"--container-architecture", "linux/amd64",
		"--input", "alpha=hello world",
		"--input", "zeta=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildArgv_AlwaysForcesLinuxAmd64_MatchesRealGitHubRunners(t *testing.T) {
	// GitHub's own ubuntu-latest runners are always linux/amd64. Without
	// forcing this, act defaults to the host's native architecture — on
	// Apple Silicon that's linux/arm64, which silently diverges from CI:
	// some native npm packages (e.g. ibm_db) refuse to install on arm64
	// even though they build fine on GitHub's real x64 runners.
	req := RunRequest{WorkflowFile: "ci.yml", Event: "push"}
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env", "/tmp/e.env", "")
	found := false
	for i, arg := range got {
		if arg == "--container-architecture" {
			found = true
			if i+1 >= len(got) || got[i+1] != "linux/amd64" {
				t.Fatalf("--container-architecture value: got %v, want linux/amd64", got)
			}
		}
	}
	if !found {
		t.Fatal("expected --container-architecture flag in argv")
	}
}

func TestBuildArgv_EnvFileAlwaysPointsAtEmptyFile_NeverRealRepoDotenv(t *testing.T) {
	// Regression guard: act defaults to reading ./.env from its working
	// directory (cmd.Dir = repoPath). Without an explicit --env-file, act
	// would silently load the target repo's real .env — mixing uncontrolled
	// secrets into the run and, on any parse error, dumping that file's
	// full contents into the log. --env-file must always be present and
	// never equal to a path literally named ".env" in the repo itself.
	req := RunRequest{WorkflowFile: "ci.yml", Event: "push"}
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env", "/tmp/local-action-empty.env", "")
	found := false
	for i, arg := range got {
		if arg == "--env-file" {
			found = true
			if i+1 >= len(got) || got[i+1] != "/tmp/local-action-empty.env" {
				t.Fatalf("--env-file value: got %v, want /tmp/local-action-empty.env", got)
			}
		}
	}
	if !found {
		t.Fatal("expected --env-file flag in argv")
	}
}
