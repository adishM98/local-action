package app

import (
	"fmt"
	"sort"
)

type RunRequest struct {
	RepoPath     string            `json:"repoPath"`
	WorkflowFile string            `json:"workflowFile"`
	Event        string            `json:"event"`
	Inputs       map[string]string `json:"inputs"`
	ExtraSecrets map[string]string `json:"extraSecrets"`
	ExtraVars    map[string]string `json:"extraVars"`
}

// BuildArgv always sets --env-file explicitly (envFile should be an empty
// file local-action controls) so act never falls back to auto-loading a
// ".env" from its working directory — which is the target repo itself.
// Letting that happen would mix uncontrolled repo secrets into the run and,
// on any parse error, dump that file's full contents into the log.
func BuildArgv(req RunRequest, secretFile, varFile, envFile string) []string {
	argv := []string{
		req.Event, "-W", req.WorkflowFile, "--json",
		"--secret-file", secretFile, "--var-file", varFile, "--env-file", envFile,
	}

	keys := make([]string, 0, len(req.Inputs))
	for k := range req.Inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		argv = append(argv, "--input", fmt.Sprintf("%s=%s", k, req.Inputs[k]))
	}
	return argv
}
