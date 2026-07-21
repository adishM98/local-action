package app

import (
	"reflect"
	"testing"
)

func TestBuildArgv_NoInputs(t *testing.T) {
	req := RunRequest{WorkflowFile: ".github/workflows/ci.yml", Event: "push"}
	got := BuildArgv(req, "/tmp/secrets.env", "/tmp/vars.env")
	want := []string{"push", "-W", ".github/workflows/ci.yml", "--secret-file", "/tmp/secrets.env", "--var-file", "/tmp/vars.env"}
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
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env")
	want := []string{
		"workflow_dispatch", "-W", "deploy.yml", "--secret-file", "/tmp/s.env", "--var-file", "/tmp/v.env",
		"--input", "alpha=hello world",
		"--input", "zeta=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
