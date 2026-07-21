package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type WorkflowInfo struct {
	File           string          `json:"file"`
	Name           string          `json:"name"`
	Events         []string        `json:"events"`
	DispatchInputs []DispatchInput `json:"dispatchInputs,omitempty"`
	ParseError     string          `json:"parseError,omitempty"`
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
	dir := filepath.Join(repoPath, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var results []WorkflowInfo
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

	var onNode *yaml.Node
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
		}
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
