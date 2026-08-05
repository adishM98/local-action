package workflows

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type WorkflowInfo struct {
	File             string          `json:"file"`
	Name             string          `json:"name"`
	Events           []string        `json:"events"`
	DispatchInputs   []DispatchInput `json:"dispatchInputs,omitempty"`
	UsedSecrets      []string        `json:"usedSecrets,omitempty"`
	UsedVars         []string        `json:"usedVars,omitempty"`
	AutoEventPayload string          `json:"autoEventPayload,omitempty"`
	// NeedsEventPayload is true when some job's if: condition references
	// github.event.* but couldn't be auto-solved into AutoEventPayload —
	// the frontend uses this to show a manual event-payload field only
	// when one is actually load-bearing, not for every workflow whose
	// condition (if any) simply doesn't depend on event data.
	NeedsEventPayload bool `json:"needsEventPayload,omitempty"`
	// SuggestedEventPayload is a best-effort guess for the manual field's
	// starting value, set only when NeedsEventPayload is true and
	// AutoEventPayload couldn't be fully solved. Unlike AutoEventPayload
	// it's not guaranteed correct — it merges whatever recognizable
	// clauses it found regardless of &&/||, so the user still needs to
	// check it.
	SuggestedEventPayload string `json:"suggestedEventPayload,omitempty"`
	// SuggestedLabels lists the (label, action) options (deduped, first-seen
	// order) this workflow's if: conditions check for — either the
	// contains(github.event.pull_request.labels.*.name, 'x') idiom, or the
	// github.event.label.name == 'x' idiom (the "labeled"/"unlabeled"
	// webhook event) — set only when exactly one of those idioms is the
	// SOLE recognized pattern across every job (see collectSuggestedLabels).
	// The same label name can legitimately appear twice with different
	// Action values (e.g. a "suspend" job reacting to it being added, a
	// "resume" job reacting to it being removed) — that's two distinct
	// options, not a duplicate, since they trigger different jobs. The
	// frontend uses this to offer a "pick a label" dropdown instead of the
	// raw JSON payload field; a workflow whose condition mixes label checks
	// with anything else, or mixes both idioms, falls back to the general
	// JSON field instead.
	SuggestedLabels []LabelOption `json:"suggestedLabels,omitempty"`
	// SuggestedLabelShape says which idiom SuggestedLabels came from, so the
	// frontend knows what shape of payload a picked option needs to become:
	// "prLabels" -> {"pull_request":{"labels":[{"name":"<label>"}]}}
	// "issueLabels" -> {"issue":{"labels":[{"name":"<label>"}]}}
	// "eventLabel" -> {"action":"<action>","label":{"name":"<label>"}}
	// prLabels and issueLabels are GitHub's identical labels-array idiom,
	// just under different root objects (pull_request vs issue — the same
	// idiom works on issues/issue_comment triggers, per GitHub's webhook
	// payload docs); a workflow mixing both still can't be represented by
	// one payload and falls back to the general JSON field like any other
	// unrecognized mix. Empty whenever SuggestedLabels is empty.
	SuggestedLabelShape string `json:"suggestedLabelShape,omitempty"`
	// IncompatibleRunners lists any runs-on labels (deduped, sorted) that
	// act cannot actually honor: act only ever runs Linux containers, so
	// windows-*/macos-*/self-hosted labels silently either fail or fall
	// back to a Linux image that doesn't match what the label claims.
	// Matrix-templated runs-on (e.g. ${{ matrix.os }}) can't be resolved
	// statically and is skipped rather than guessed at.
	IncompatibleRunners []string `json:"incompatibleRunners,omitempty"`
	// Jobs describes this workflow's job dependency structure (id, display
	// name, needs) — the frontend joins this static shape against a run's
	// live per-job status (already reconstructed from act's JSON log
	// lines, see web/src/logparse.js) to draw the job graph. Order matches
	// declaration order in the YAML.
	Jobs         []JobInfo `json:"jobs,omitempty"`
	AutoCategory string    `json:"autoCategory"`
	ParseError   string    `json:"parseError,omitempty"`
}

// JobInfo is one job's static structure — id is the YAML key (what a
// sibling job's needs: entry references, and the same identity act's
// jobID log field uses), name is the human-readable label (defaults to id
// when the job has no name: of its own). Line is the 1-based line where the
// job's key appears in the workflow file, used to locate its YAML block
// when the frontend points at the source for a failed job.
type JobInfo struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Needs []string   `json:"needs,omitempty"`
	Line  int        `json:"line,omitempty"`
	Steps []StepInfo `json:"steps,omitempty"`
}

// StepInfo is one step's static position — Name is the step's own `name:`
// (empty when the step doesn't declare one, e.g. a bare `run:`/`uses:`
// with no name — act then falls back to a synthesized label the frontend
// can't reliably match back to source, so those steps only contribute
// their Line as a boundary for locating neighboring named steps).
type StepInfo struct {
	Name string `json:"name,omitempty"`
	Line int    `json:"line"`
}

// categoryKeywords is checked in order — more specific categories (Security)
// must win over generic ones (CI/Build) even when a name contains both
// (e.g. "Grype - Docker Image Vulnerability Scan" contains "docker" but is
// clearly a security scan, not a build).
var categoryKeywords = []struct {
	category string
	keywords []string
}{
	{"Security", []string{"security", "vulnerab", "licen", "compliance", "grype", "cve"}},
	{"Deployment", []string{"deploy", "publish", "render", "netlify"}},
	{"Testing", []string{"test", "cypress", "coverage", "e2e"}},
	{"Docs", []string{"docs", "storybook"}},
	{"CI/Build", []string{"build", "docker", "packer", "ami", "image"}},
}

var ciWordRe = regexp.MustCompile(`\bci\b`)

// autoCategoryFor guesses a sidebar category from a workflow's name and file
// path. Falls back to "Other" when nothing matches — never hides a workflow.
func autoCategoryFor(name, file string) string {
	haystack := strings.ToLower(name + " " + file)
	for _, c := range categoryKeywords {
		for _, kw := range c.keywords {
			if strings.Contains(haystack, kw) {
				return c.category
			}
		}
	}
	if ciWordRe.MatchString(haystack) {
		return "CI/Build"
	}
	return "Other"
}

var (
	secretRefRe = regexp.MustCompile(`\$\{\{\s*secrets\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
	varRefRe    = regexp.MustCompile(`\$\{\{\s*vars\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
)

// findRefs returns the deduped, sorted set of names captured by re's first
// group across data, skipping any name in exclude.
func findRefs(re *regexp.Regexp, data []byte, exclude map[string]bool) []string {
	seen := map[string]bool{}
	var names []string
	for _, m := range re.FindAllSubmatch(data, -1) {
		name := string(m[1])
		if exclude[name] || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type DispatchInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
}

func ScanWorkflows(repoPath string) ([]WorkflowInfo, error) {
	info, err := os.Stat(repoPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("repo path does not exist: %s", repoPath)
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repo path must be a directory (the repo root), not a file: %s", repoPath)
	}

	dir := filepath.Join(repoPath, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []WorkflowInfo{}, nil
	}
	if err != nil {
		return nil, err
	}

	results := []WorkflowInfo{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		info, err := ParseWorkflowFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		info.File = filepath.Join(".github", "workflows", name)
		results = append(results, info)
	}
	return results, nil
}

// ReadWorkflowSource returns the raw YAML for a workflow file, for the
// frontend's "view source" panel. workflowFile is client-supplied (a repo
// path picked from the scan results), so it's resolved and re-checked
// against the workflows dir rather than trusted outright — otherwise a
// crafted "../../etc/passwd"-style value could read any file the process
// can see.
func ReadWorkflowSource(repoPath, workflowFile string) (string, error) {
	dir := filepath.Clean(filepath.Join(repoPath, ".github", "workflows"))
	full := filepath.Clean(filepath.Join(repoPath, workflowFile))
	if full != dir && !strings.HasPrefix(full, dir+string(filepath.Separator)) {
		return "", fmt.Errorf("workflow file is outside the workflows directory: %s", workflowFile)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ParseWorkflowFile(path string) (WorkflowInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowInfo{}, err
	}
	info := WorkflowInfo{Name: filepath.Base(path)}
	info.UsedSecrets = findRefs(secretRefRe, data, map[string]bool{"GITHUB_TOKEN": true})
	info.UsedVars = findRefs(varRefRe, data, nil)
	info.AutoCategory = autoCategoryFor(info.Name, filepath.Base(path))

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		info.ParseError = err.Error()
		return info, nil
	}
	if len(doc.Content) == 0 {
		info.ParseError = "empty workflow file"
		return info, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		info.ParseError = "workflow file root is not a mapping"
		return info, nil
	}

	var onNode, jobsNode *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]
		switch keyNode.Value {
		case "name":
			if valNode.Kind == yaml.ScalarNode {
				info.Name = valNode.Value
			}
		case "on":
			onNode = valNode
		case "jobs":
			jobsNode = valNode
		}
	}

	info.AutoCategory = autoCategoryFor(info.Name, filepath.Base(path))

	if jobsNode != nil {
		info.AutoEventPayload, info.SuggestedEventPayload, info.NeedsEventPayload = autoEventPayloadFromJobs(jobsNode)
		info.SuggestedLabels, info.SuggestedLabelShape = collectSuggestedLabels(jobsNode)
		info.IncompatibleRunners = incompatibleRunners(jobsNode)
		info.Jobs = parseJobs(jobsNode)
	}

	if onNode == nil {
		info.ParseError = "no 'on' trigger block found"
		return info, nil
	}

	events, dispatchInputs, err := parseOnNode(onNode)
	if err != nil {
		info.ParseError = err.Error()
		return info, nil
	}
	info.Events = events
	info.DispatchInputs = dispatchInputs
	return info, nil
}

func parseOnNode(n *yaml.Node) ([]string, []DispatchInput, error) {
	var events []string
	var dispatchInputs []DispatchInput

	switch n.Kind {
	case yaml.ScalarNode:
		events = append(events, n.Value)
	case yaml.SequenceNode:
		for _, item := range n.Content {
			events = append(events, item.Value)
		}
	case yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			valNode := n.Content[i+1]
			events = append(events, keyNode.Value)
			if keyNode.Value == "workflow_dispatch" && valNode.Kind == yaml.MappingNode {
				dispatchInputs = parseDispatchInputs(valNode)
			}
		}
	default:
		return nil, nil, fmt.Errorf("unsupported 'on' node kind: %v", n.Kind)
	}
	return events, dispatchInputs, nil
}

var (
	// Plain equality: github.event.<path> == 'value'.
	ifClauseRe = regexp.MustCompile(`github\.event\.([A-Za-z0-9_.]+)\s*==\s*(?:'([^']*)'|"([^"]*)")`)
	// The common "does this PR/issue have label X" idiom: GitHub's `.*.`
	// glob selects a field across every element of an array and flattens it
	// into a list, which contains() then searches — e.g.
	// contains(github.event.pull_request.labels.*.name, 'run-ci'). Per
	// GitHub's webhook payload docs, issues/issue_comment triggers carry
	// the identical shape under github.event.issue.labels instead — the
	// regex matches either root; collectSuggestedLabels is what decides
	// which one is actually present and disqualifies on a mix of both.
	containsLabelRe = regexp.MustCompile(`contains\(\s*github\.event\.([A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*)\.\*\.([A-Za-z0-9_]+)\s*,\s*(?:'([^']*)'|"([^"]*)")\s*\)`)
	githubEventRe   = regexp.MustCompile(`github\.event\.`)
)

// isNegatedAt reports whether the character immediately before cond[idx]
// (skipping whitespace) is "!" — i.e. whether the clause starting at idx is
// negated, e.g. !contains(...). A negated contains() means "does NOT have
// this label"; matching it the same as the positive form would suggest a
// label that, if picked, makes the condition FALSE instead of true — the
// exact inverse of what picking a label from the dropdown is supposed to
// do. Doesn't handle "!=" (a different token, already excluded since
// ifClauseRe only matches "==") or "!(...)" wrapping a whole parenthesized
// group — both real but rarer than a bare !contains(...).
func isNegatedAt(cond string, idx int) bool {
	i := idx - 1
	for i >= 0 && (cond[i] == ' ' || cond[i] == '\t') {
		i--
	}
	return i >= 0 && cond[i] == '!'
}

// positiveMatches runs re against cond like FindAllStringSubmatch, minus any
// match that's actually negated (see isNegatedAt) — every caller that cares
// about matching a POSITIVE "this is true" check should use this instead of
// calling the regex directly.
func positiveMatches(re *regexp.Regexp, cond string) [][]string {
	idxMatches := re.FindAllSubmatchIndex([]byte(cond), -1)
	var out [][]string
	for _, idx := range idxMatches {
		if isNegatedAt(cond, idx[0]) {
			continue
		}
		groups := make([]string, len(idx)/2)
		for g := 0; g < len(idx)/2; g++ {
			if idx[2*g] >= 0 {
				groups[g] = cond[idx[2*g]:idx[2*g+1]]
			}
		}
		out = append(out, groups)
	}
	return out
}

func containsLabelMatches(cond string) [][]string { return positiveMatches(containsLabelRe, cond) }
func ifClauseMatches(cond string) [][]string       { return positiveMatches(ifClauseRe, cond) }

// autoEventPayloadFromJobs merges the recognizable clauses from EVERY job's
// if: condition (not just the first) into one payload — a workflow's jobs
// commonly gate on different label sets (e.g. a build job on "run-cypress",
// a deploy job on "run-cypress-deployments"), so looking at only one job
// would silently drop the rest. The merged payload is returned as the
// confident payload (act's -e/--eventpath shape, ready to use without
// disclaimer) only when EVERY scanned condition fully solves on its own
// (see solveIfCondition) — if even one job's condition can't be fully
// solved (e.g. it uses ||), the whole merged result is returned as
// suggested instead: using just the one job that happens to solve trivially
// would both hide the manual-entry field the other jobs still need AND
// throw away their labels. needsPayload is true whenever a condition
// references github.event.* at all, solved or not.
func autoEventPayloadFromJobs(jobsNode *yaml.Node) (payload, suggested string, needsPayload bool) {
	if jobsNode.Kind != yaml.MappingNode {
		return "", "", false
	}
	root := map[string]any{}
	found := false
	allSolved := true
	for i := 0; i < len(jobsNode.Content); i += 2 {
		jobNode := jobsNode.Content[i+1]
		if jobNode.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(jobNode.Content); j += 2 {
			if jobNode.Content[j].Value != "if" {
				continue
			}
			ifValue := jobNode.Content[j+1]
			if ifValue.Kind != yaml.ScalarNode {
				continue
			}
			if !githubEventRe.MatchString(ifValue.Value) {
				continue
			}
			needsPayload = true
			if _, ok := solveIfCondition(ifValue.Value); !ok {
				allSolved = false
			}
			if mergeEventPayloadClauses(root, ifValue.Value) {
				found = true
			}
		}
	}
	if !found {
		return "", "", needsPayload
	}
	b, err := json.Marshal(root)
	if err != nil {
		return "", "", needsPayload
	}
	if allSolved {
		return string(b), "", true
	}
	return "", string(b), true
}

// LabelOption is one label a workflow's if: conditions check for, paired
// with the action value ("labeled"/"unlabeled") it needs to actually
// trigger the job that checks it — see collectSuggestedLabels. Action is
// always empty for the prLabels idiom, which has no action field at all.
type LabelOption struct {
	Label  string `json:"label"`
	Action string `json:"action,omitempty"`
}

// collectSuggestedLabels scans every job's if: condition for either of two
// label-checking idioms and returns the deduped (label, action) options in
// first-seen order, plus which idiom they came from. The same label name
// can legitimately produce two different options (e.g. a "suspend" job
// reacting to it being added, a "resume" job reacting to it being
// removed) — picking the wrong action would build a payload that silently
// doesn't trigger the job the user meant to simulate, so those must stay
// distinct rather than collapsing into one.
//
// Pairing an eventLabel match's action is positional: within one
// condition, the action value most recently seen before a label.name match
// is that label's action — matching how these conditions are actually
// written (action check first, e.g. "action == 'labeled' && (label.name ==
// 'a' || label.name == 'b')"). An action clause with no label.name
// anywhere in the same condition (e.g. "... || action == 'closed'", a
// branch with nothing to do with any label) is simply skipped rather than
// disqualifying — the dropdown just can't simulate that branch, same as it
// never claimed to cover every possible trigger.
//
// The two idioms, and any other clause, disqualify the whole workflow
// under the same rule as before: only recognized when it's the SOLE
// pattern used, and the workflow doesn't mix both label idioms across
// jobs (a single dropdown can't represent two different payload shapes).
func collectSuggestedLabels(jobsNode *yaml.Node) (options []LabelOption, shape string) {
	if jobsNode.Kind != yaml.MappingNode {
		return nil, ""
	}

	seen := map[LabelOption]bool{}
	sawOther := false
	setShape := func(s string) {
		if shape != "" && shape != s {
			sawOther = true // mixing both idioms — no single payload shape fits
			return
		}
		shape = s
	}
	add := func(opt LabelOption) {
		if !seen[opt] {
			seen[opt] = true
			options = append(options, opt)
		}
	}

	for i := 0; i < len(jobsNode.Content); i += 2 {
		jobNode := jobsNode.Content[i+1]
		if jobNode.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(jobNode.Content); j += 2 {
			if jobNode.Content[j].Value != "if" {
				continue
			}
			ifValue := jobNode.Content[j+1]
			if ifValue.Kind != yaml.ScalarNode {
				continue
			}
			cond := ifValue.Value
			if !githubEventRe.MatchString(cond) {
				continue
			}
			ifMatches := ifClauseMatches(cond)
			// github.event.action == '...' is only a safe companion clause
			// alongside label.name checks (the real shape of a "labeled"/
			// "unlabeled" webhook event) — the same clause next to the
			// contains(pull_request.labels...) idiom (no label.name
			// present) is a genuinely separate requirement a label-only
			// payload can't satisfy, so it must still disqualify there.
			isEventLabelCond := false
			for _, m := range ifMatches {
				if m[1] == "label.name" {
					isEventLabelCond = true
					break
				}
			}
			lastAction := ""
			for _, m := range ifMatches {
				value := m[2]
				if value == "" && m[3] != "" {
					value = m[3]
				}
				switch {
				case m[1] == "label.name":
					setShape("eventLabel")
					add(LabelOption{Label: value, Action: lastAction})
				case m[1] == "action" && isEventLabelCond:
					lastAction = value
				default:
					sawOther = true
				}
			}
			for _, m := range containsLabelMatches(cond) {
				var shapeForPath string
				switch m[1] {
				case "pull_request.labels":
					shapeForPath = "prLabels"
				case "issue.labels":
					shapeForPath = "issueLabels"
				}
				if shapeForPath == "" || m[2] != "name" {
					sawOther = true
					continue
				}
				value := m[3]
				if value == "" && m[4] != "" {
					value = m[4]
				}
				setShape(shapeForPath)
				add(LabelOption{Label: value})
			}
		}
	}

	if sawOther || len(options) == 0 {
		return nil, ""
	}
	return options, shape
}

// solveIfCondition fully solves cond only when it's a plain conjunction
// (&&-joined, no ||, no negation) of clauses this package recognizes:
// github.event.<path> == 'value', or contains(github.event.<path>.*.<leaf>,
// 'value'). Anything else — a single unrecognized clause, or any use of ||
// or ! anywhere in cond — fails the whole condition, since a partial solve
// of a conjunction could silently produce a payload that makes the
// condition evaluate true when the real event wouldn't.
func solveIfCondition(cond string) (string, bool) {
	cond = strings.TrimSpace(cond)
	cond = strings.TrimPrefix(cond, "${{")
	cond = strings.TrimSuffix(cond, "}}")
	cond = strings.TrimSpace(cond)
	if cond == "" || strings.Contains(cond, "||") || strings.Contains(cond, "!") {
		return "", false
	}

	root := map[string]any{}
	for _, clause := range strings.Split(cond, "&&") {
		clause = strings.TrimSpace(clause)
		if !applyClause(root, clause, true) {
			return "", false
		}
	}

	b, err := json.Marshal(root)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// mergeEventPayloadClauses scans cond for every recognizable clause
// regardless of how they're joined (&&, ||, or anything else) and merges
// them into root, accumulating rather than overwriting — a single condition
// commonly ||-checks several labels (contains(..., 'a') || contains(...,
// 'b')), and a workflow commonly has several jobs each gating on a
// different label, so root is shared across every call for the whole
// workflow and every recognized label ends up present at once. Unlike
// solveIfCondition this never fails outright; it returns false only when
// nothing recognizable was found in cond.
func mergeEventPayloadClauses(root map[string]any, cond string) bool {
	cond = strings.TrimSpace(cond)
	cond = strings.TrimPrefix(cond, "${{")
	cond = strings.TrimSuffix(cond, "}}")
	cond = strings.TrimSpace(cond)

	found := false
	for _, m := range ifClauseMatches(cond) {
		value := m[2]
		if value == "" && m[3] != "" {
			value = m[3]
		}
		setNestedPath(root, strings.Split(m[1], "."), value)
		found = true
	}
	for _, m := range containsLabelMatches(cond) {
		value := m[3]
		if value == "" && m[4] != "" {
			value = m[4]
		}
		appendNestedArrayLeaf(root, m[1], m[2], value)
		found = true
	}
	return found
}

// applyClause matches a single clause against every recognized pattern and
// applies the first match to root. anchored requires the clause to consist
// of nothing but the matched pattern (used by solveIfCondition, where a
// clause with trailing junk isn't actually fully understood); the
// permissive scan in mergeEventPayloadClauses doesn't call this at all since
// it wants every match anywhere, anchored or not.
func applyClause(root map[string]any, clause string, anchored bool) bool {
	if m := ifClauseRe.FindStringSubmatch(clause); m != nil && (!anchored || m[0] == clause) {
		value := m[2]
		if value == "" && m[3] != "" {
			value = m[3]
		}
		setNestedPath(root, strings.Split(m[1], "."), value)
		return true
	}
	if m := containsLabelRe.FindStringSubmatch(clause); m != nil && (!anchored || m[0] == clause) {
		value := m[3]
		if value == "" && m[4] != "" {
			value = m[4]
		}
		appendNestedArrayLeaf(root, m[1], m[2], value)
		return true
	}
	return false
}

func setNestedPath(root map[string]any, path []string, value string) {
	node := root
	for _, key := range path[:len(path)-1] {
		next, ok := node[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[key] = next
		}
		node = next
	}
	node[path[len(path)-1]] = value
}

// appendNestedArrayLeaf handles the contains(...*.leaf...) shape: basePath's
// last segment is the array field itself (e.g. "pull_request.labels" ->
// object nesting "pull_request", then an array field "labels"). It appends
// {leaf: value} to that array rather than replacing it, since a workflow
// commonly checks several distinct label values across ||-joined clauses or
// separate jobs (run-cypress, run-cypress-ce, run-cypress-ce-deployments,
// ...) — a single-element array would silently drop every label but the
// last one seen, which acts `.*.` glob-and-flatten then can't match against.
func appendNestedArrayLeaf(root map[string]any, basePath, leaf, value string) {
	segments := strings.Split(basePath, ".")
	arrayField := segments[len(segments)-1]
	node := root
	for _, key := range segments[:len(segments)-1] {
		next, ok := node[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[key] = next
		}
		node = next
	}
	existing, _ := node[arrayField].([]any)
	for _, e := range existing {
		if m, ok := e.(map[string]any); ok && m[leaf] == value {
			return
		}
	}
	node[arrayField] = append(existing, map[string]any{leaf: value})
}

var incompatibleRunnerRe = regexp.MustCompile(`(?i)^(windows|macos|self-hosted)`)

// incompatibleRunners scans every job's runs-on for labels act can't
// actually honor, deduped and sorted.
func incompatibleRunners(jobsNode *yaml.Node) []string {
	if jobsNode.Kind != yaml.MappingNode {
		return nil
	}
	seen := map[string]bool{}
	var found []string
	for i := 0; i < len(jobsNode.Content); i += 2 {
		jobNode := jobsNode.Content[i+1]
		if jobNode.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(jobNode.Content); j += 2 {
			if jobNode.Content[j].Value != "runs-on" {
				continue
			}
			for _, label := range runsOnLabels(jobNode.Content[j+1]) {
				if incompatibleRunnerRe.MatchString(label) && !seen[label] {
					seen[label] = true
					found = append(found, label)
				}
			}
		}
	}
	sort.Strings(found)
	return found
}

// parseJobs walks jobsNode into JobInfo entries, in declaration order —
// the id/needs identity the job graph draws edges from.
func parseJobs(jobsNode *yaml.Node) []JobInfo {
	if jobsNode.Kind != yaml.MappingNode {
		return nil
	}
	var jobs []JobInfo
	for i := 0; i < len(jobsNode.Content); i += 2 {
		id := jobsNode.Content[i].Value
		jobNode := jobsNode.Content[i+1]
		job := JobInfo{ID: id, Name: id, Line: jobsNode.Content[i].Line}
		if jobNode.Kind == yaml.MappingNode {
			for j := 0; j < len(jobNode.Content); j += 2 {
				switch jobNode.Content[j].Value {
				case "name":
					if v := jobNode.Content[j+1]; v.Kind == yaml.ScalarNode && v.Value != "" {
						job.Name = v.Value
					}
				case "needs":
					job.Needs = needsList(jobNode.Content[j+1])
				case "steps":
					job.Steps = parseSteps(jobNode.Content[j+1])
				}
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

// parseSteps records each step's declared name (if any) and the line its
// sequence item starts on — the frontend uses consecutive steps' lines to
// bound each step's YAML block when highlighting the one that failed.
func parseSteps(stepsNode *yaml.Node) []StepInfo {
	if stepsNode == nil || stepsNode.Kind != yaml.SequenceNode {
		return nil
	}
	var steps []StepInfo
	for _, stepNode := range stepsNode.Content {
		step := StepInfo{Line: stepNode.Line}
		if stepNode.Kind == yaml.MappingNode {
			for j := 0; j < len(stepNode.Content); j += 2 {
				if stepNode.Content[j].Value == "name" {
					if v := stepNode.Content[j+1]; v.Kind == yaml.ScalarNode {
						step.Name = v.Value
					}
				}
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// needsList normalizes needs:'s two shapes (a single job id, or a list of
// them) into a flat list.
func needsList(n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var needs []string
		for _, item := range n.Content {
			if item.Kind == yaml.ScalarNode {
				needs = append(needs, item.Value)
			}
		}
		return needs
	}
	return nil
}

// runsOnLabels normalizes every runs-on shape (scalar, list, or the
// {group, labels} map form) into a flat label list. A matrix-templated
// value (contains ${{) is skipped — it can't be resolved without
// expanding the matrix, and act does that expansion itself at run time.
func runsOnLabels(n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		if strings.Contains(n.Value, "${{") {
			return nil
		}
		return []string{n.Value}
	case yaml.SequenceNode:
		var labels []string
		for _, item := range n.Content {
			if item.Kind == yaml.ScalarNode && !strings.Contains(item.Value, "${{") {
				labels = append(labels, item.Value)
			}
		}
		return labels
	case yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			if n.Content[i].Value == "labels" {
				return runsOnLabels(n.Content[i+1])
			}
		}
	}
	return nil
}

func parseDispatchInputs(n *yaml.Node) []DispatchInput {
	var inputs []DispatchInput
	for i := 0; i < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		valNode := n.Content[i+1]
		if keyNode.Value != "inputs" || valNode.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(valNode.Content); j += 2 {
			nameNode := valNode.Content[j]
			specNode := valNode.Content[j+1]
			input := DispatchInput{Name: nameNode.Value, Type: "string"}
			if specNode.Kind == yaml.MappingNode {
				for k := 0; k < len(specNode.Content); k += 2 {
					fk := specNode.Content[k].Value
					fv := specNode.Content[k+1]
					switch fk {
					case "description":
						input.Description = fv.Value
					case "required":
						input.Required = fv.Value == "true"
					case "default":
						input.Default = fv.Value
					case "type":
						input.Type = fv.Value
					case "options":
						for _, opt := range fv.Content {
							input.Options = append(input.Options, opt.Value)
						}
					}
				}
			}
			inputs = append(inputs, input)
		}
	}
	return inputs
}
