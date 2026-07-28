// Package proxytemplate parses the constrained upstream-proxy credential templates.
package proxytemplate

import (
	"errors"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
)

// ErrUnsupportedFunction reports a function outside the fixed proxy template allowlist.
var ErrUnsupportedFunction = errors.New("unsupported proxy template function")

// Parse parses a proxy credential template with only the supported functions.
func Parse(name, raw string) (*template.Template, error) {
	parsed, err := template.New(name).
		Option("missingkey=error").
		Funcs(template.FuncMap{"lower": strings.ToLower, "upper": strings.ToUpper}).
		Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse proxy template: %w", err)
	}

	for _, candidate := range parsed.Templates() {
		if !validNode(candidate.Root) {
			return nil, ErrUnsupportedFunction
		}
	}

	return parsed, nil
}

func validNode(node parse.Node) bool {
	switch node := node.(type) {
	case nil:
		return true
	case *parse.ListNode:
		return validList(node)
	case *parse.ActionNode:
		return validNode(node.Pipe)
	case *parse.IfNode:
		return validBranch(node.Pipe, node.List, node.ElseList)
	case *parse.RangeNode:
		return validBranch(node.Pipe, node.List, node.ElseList)
	case *parse.WithNode:
		return validBranch(node.Pipe, node.List, node.ElseList)
	default:
		return validValue(node)
	}
}

func validList(node *parse.ListNode) bool {
	if node == nil {
		return true
	}

	for _, child := range node.Nodes {
		if !validNode(child) {
			return false
		}
	}

	return true
}

func validBranch(pipe *parse.PipeNode, list, elseList *parse.ListNode) bool {
	return validNode(pipe) && validNode(list) && validNode(elseList)
}

func validValue(node parse.Node) bool {
	switch node := node.(type) {
	case *parse.TemplateNode:
		return validNode(node.Pipe)
	case *parse.PipeNode:
		return validCommands(node.Cmds)
	case *parse.CommandNode:
		return validArguments(node.Args)
	case *parse.ChainNode:
		return validNode(node.Node)
	case *parse.IdentifierNode:
		return node.Ident == "lower" || node.Ident == "upper"
	default:
		return true
	}
}

func validCommands(commands []*parse.CommandNode) bool {
	for _, command := range commands {
		if !validNode(command) {
			return false
		}
	}

	return true
}

func validArguments(arguments []parse.Node) bool {
	for _, argument := range arguments {
		if !validNode(argument) {
			return false
		}
	}

	return true
}
