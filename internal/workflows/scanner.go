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
	AutoCategory     string          `json:"autoCategory"`
	ParseError       string          `json:"parseError,omitempty"`
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
		info.AutoEventPayload = autoEventPayloadFromJobs(jobsNode)
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

var ifClauseRe = regexp.MustCompile(`^github\.event\.([A-Za-z0-9_.]+)\s*==\s*(?:'([^']*)'|"([^"]*)")$`)

// autoEventPayloadFromJobs scans each job's if: condition for the first one
// that reduces to a plain conjunction of github.event.<path> == '<value>'
// clauses, and returns the matching JSON payload (act's -e/--eventpath
// shape) so the condition evaluates true without the user hand-writing it.
// Conditions using ||, negation, function calls, or comparisons against
// anything other than github.event.* are left alone — job.if is simply not
// auto-solvable, and the user can still supply a payload manually.
func autoEventPayloadFromJobs(jobsNode *yaml.Node) string {
	if jobsNode.Kind != yaml.MappingNode {
		return ""
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
			if payload, ok := solveIfCondition(ifValue.Value); ok {
				return payload
			}
		}
	}
	return ""
}

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
		m := ifClauseRe.FindStringSubmatch(strings.TrimSpace(clause))
		if m == nil {
			return "", false
		}
		value := m[2]
		if value == "" && m[3] != "" {
			value = m[3]
		}
		setNestedPath(root, strings.Split(m[1], "."), value)
	}

	b, err := json.Marshal(root)
	if err != nil {
		return "", false
	}
	return string(b), true
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
