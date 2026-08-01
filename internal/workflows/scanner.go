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
	// IncompatibleRunners lists any runs-on labels (deduped, sorted) that
	// act cannot actually honor: act only ever runs Linux containers, so
	// windows-*/macos-*/self-hosted labels silently either fail or fall
	// back to a Linux image that doesn't match what the label claims.
	// Matrix-templated runs-on (e.g. ${{ matrix.os }}) can't be resolved
	// statically and is skipped rather than guessed at.
	IncompatibleRunners []string `json:"incompatibleRunners,omitempty"`
	AutoCategory        string   `json:"autoCategory"`
	ParseError          string   `json:"parseError,omitempty"`
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
		info.IncompatibleRunners = incompatibleRunners(jobsNode)
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
	// The common "does this PR have label X" idiom: GitHub's `.*.` glob
	// selects a field across every element of an array and flattens it
	// into a list, which contains() then searches — e.g.
	// contains(github.event.pull_request.labels.*.name, 'run-ci').
	containsLabelRe = regexp.MustCompile(`contains\(\s*github\.event\.([A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*)\.\*\.([A-Za-z0-9_]+)\s*,\s*(?:'([^']*)'|"([^"]*)")\s*\)`)
	githubEventRe   = regexp.MustCompile(`github\.event\.`)
)

// autoEventPayloadFromJobs scans each job's if: condition for the first one
// that fully solves (see solveIfCondition), and returns its JSON payload
// (act's -e/--eventpath shape) so the condition evaluates true without the
// user hand-writing it. If nothing fully solves but some condition still
// references github.event.*, suggested is a best-effort guess built from
// whichever individual clauses ARE recognizable — a starting point for
// manual entry, not a guarantee, since it ignores how clauses combine
// (&&/||) and merges every match it finds. needsPayload is true whenever a
// condition references github.event.* at all, solved or not — that's what
// tells the frontend a payload is actually load-bearing for this workflow.
func autoEventPayloadFromJobs(jobsNode *yaml.Node) (payload, suggested string, needsPayload bool) {
	if jobsNode.Kind != yaml.MappingNode {
		return "", "", false
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
			if !githubEventRe.MatchString(ifValue.Value) {
				continue
			}
			if solved, ok := solveIfCondition(ifValue.Value); ok {
				return solved, "", true
			}
			needsPayload = true
			if suggested == "" {
				suggested = suggestEventPayload(ifValue.Value)
			}
		}
	}
	return "", suggested, needsPayload
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

// suggestEventPayload scans cond for every recognizable clause regardless
// of how they're joined (&&, ||, or anything else) and merges them into
// one best-effort payload. Unlike solveIfCondition this never fails outright
// — it returns "" only when nothing recognizable was found at all.
func suggestEventPayload(cond string) string {
	cond = strings.TrimSpace(cond)
	cond = strings.TrimPrefix(cond, "${{")
	cond = strings.TrimSuffix(cond, "}}")
	cond = strings.TrimSpace(cond)

	root := map[string]any{}
	found := false
	for _, m := range ifClauseRe.FindAllStringSubmatch(cond, -1) {
		value := m[2]
		if value == "" && m[3] != "" {
			value = m[3]
		}
		setNestedPath(root, strings.Split(m[1], "."), value)
		found = true
	}
	for _, m := range containsLabelRe.FindAllStringSubmatch(cond, -1) {
		value := m[3]
		if value == "" && m[4] != "" {
			value = m[4]
		}
		setNestedArrayLeaf(root, m[1], m[2], value)
		found = true
	}
	if !found {
		return ""
	}
	b, err := json.Marshal(root)
	if err != nil {
		return ""
	}
	return string(b)
}

// applyClause matches a single clause against every recognized pattern and
// applies the first match to root. anchored requires the clause to consist
// of nothing but the matched pattern (used by solveIfCondition, where a
// clause with trailing junk isn't actually fully understood); the
// permissive scan in suggestEventPayload doesn't call this at all since it
// wants every match anywhere, anchored or not.
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
		setNestedArrayLeaf(root, m[1], m[2], value)
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

// setNestedArrayLeaf handles the contains(...*.leaf...) shape: basePath's
// last segment is the array field itself (e.g. "pull_request.labels" ->
// object nesting "pull_request", then an array field "labels"), set to a
// one-element array carrying leaf: value — just enough for act's `.*.`
// glob-and-flatten to produce [value], which contains() then matches.
func setNestedArrayLeaf(root map[string]any, basePath, leaf, value string) {
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
	node[arrayField] = []any{map[string]any{leaf: value}}
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
