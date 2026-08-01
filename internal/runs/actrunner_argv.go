package runs

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
	EventPayload string            `json:"eventPayload"`
}

// BuildArgv always sets --env-file explicitly (envFile should be an empty
// file local-action controls) so act never falls back to auto-loading a
// ".env" from its working directory — which is the target repo itself.
// Letting that happen would mix uncontrolled repo secrets into the run and,
// on any parse error, dump that file's full contents into the log.
//
// eventPayloadFile, when non-empty, is passed as act's -e/--eventpath so
// github.event.* is populated — the only way an if: condition gated on
// event context (e.g. a labeled-PR trigger) can evaluate true locally.
// Omitted entirely when empty, matching today's argv for every workflow
// that doesn't need it.
//
// --container-architecture linux/amd64 is always forced: GitHub's own
// ubuntu-latest runners are x64, but act defaults to the host's native
// architecture otherwise — on Apple Silicon that's arm64, which silently
// diverges from real CI (some native npm packages refuse to build on
// arm64 even though they work fine on GitHub's actual x64 runners).
//
// AGENT_TOOLSDIRECTORY is always overridden to a scratch path. act's
// runner image sets this to /opt/hostedtoolcache to emulate GitHub-hosted
// runners, but unlike a real hosted runner that path is an active mount
// point in act's container — busy, not just populated. A very common
// "free disk space" CI step does `rm -rf "$AGENT_TOOLSDIRECTORY"` (with no
// `|| true`, unlike the apt-get cleanup lines usually right above it in
// the same script), and `rm -rf` on a busy mount fails even with -f,
// taking the whole step down under act even though the exact same script
// runs fine on a real runner. Pointing the variable at a path that simply
// doesn't exist in the container makes that rm a silent no-op — verified
// against a real act invocation, not inferred. This never touches the
// scanned repo's workflow files.
func BuildArgv(req RunRequest, secretFile, varFile, envFile, eventPayloadFile string) []string {
	argv := []string{
		req.Event, "-W", req.WorkflowFile, "--json",
		"--secret-file", secretFile, "--var-file", varFile, "--env-file", envFile,
		"--container-architecture", "linux/amd64",
		"--env", "AGENT_TOOLSDIRECTORY=/tmp/local-action-agent-tools",
	}
	if eventPayloadFile != "" {
		argv = append(argv, "-e", eventPayloadFile)
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
