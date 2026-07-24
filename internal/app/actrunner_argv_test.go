package app

import (
	"reflect"
	"testing"
)

func TestBuildArgv_NoInputs(t *testing.T) {
	req := RunRequest{WorkflowFile: ".github/workflows/ci.yml", Event: "push"}
	got := BuildArgv(req, "/tmp/secrets.env", "/tmp/vars.env", "/tmp/empty.env")
	want := []string{
		"push", "-W", ".github/workflows/ci.yml", "--json",
		"--secret-file", "/tmp/secrets.env", "--var-file", "/tmp/vars.env", "--env-file", "/tmp/empty.env",
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
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env", "/tmp/empty.env")
	want := []string{
		"workflow_dispatch", "-W", "deploy.yml", "--json",
		"--secret-file", "/tmp/s.env", "--var-file", "/tmp/v.env", "--env-file", "/tmp/empty.env",
		"--input", "alpha=hello world",
		"--input", "zeta=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
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
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env", "/tmp/local-action-empty.env")
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
