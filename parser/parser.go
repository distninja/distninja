package parser

import (
	"fmt"
	"strings"

	"github.com/cayleygraph/quad"

	"github.com/distninja/distninja/store"
)

// ParsedBuild represents a parsed build statement before it's stored
type ParsedBuild struct {
	Rule         string
	Outputs      []string
	Inputs       []string
	ImplicitDeps []string
	OrderDeps    []string
	Variables    map[string]string
	Pool         string
}

// NinjaParser handles parsing of Ninja build files
type NinjaParser struct {
	store *store.NinjaStore
}

// NewNinjaParser creates a new parser instance
func NewNinjaParser(ninjaStore *store.NinjaStore) *NinjaParser {
	return &NinjaParser{
		store: ninjaStore,
	}
}

// ParseAndLoad parses ninja file content and loads it into the store
func (p *NinjaParser) ParseAndLoad(content string) error {
	lines := strings.Split(content, "\n")

	var currentRule *store.NinjaRule
	var currentBuild *ParsedBuild

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle line continuations
		for strings.HasSuffix(line, "$") && i+1 < len(lines) {
			i++
			if i < len(lines) {
				line = line[:len(line)-1] + " " + strings.TrimSpace(lines[i])
			}
		}

		// Parse rule definitions
		if strings.HasPrefix(line, "rule ") {
			// Save previous rule if exists and it's complete
			if currentRule != nil {
				if currentRule.Command == "" {
					return fmt.Errorf("rule %s is missing required command", currentRule.Name)
				}
				if _, err := p.store.AddRule(currentRule); err != nil {
					return fmt.Errorf("failed to add rule %s: %w", currentRule.Name, err)
				}
			}

			ruleName := strings.TrimSpace(line[5:])
			currentRule = &store.NinjaRule{
				Name:      ruleName,
				Variables: "{}",
			}
			continue
		}

		// Parse build statements
		if strings.HasPrefix(line, "build ") {
			// Save previous rule if exists and it's complete
			if currentRule != nil {
				if currentRule.Command == "" {
					return fmt.Errorf("rule %s is missing required command", currentRule.Name)
				}
				if _, err := p.store.AddRule(currentRule); err != nil {
					return fmt.Errorf("failed to add rule %s: %w", currentRule.Name, err)
				}
				currentRule = nil
			}

			// Save previous build if exists
			if currentBuild != nil {
				if err := p.saveBuild(currentBuild); err != nil {
					return fmt.Errorf("failed to save build: %w", err)
				}
			}

			// Parse build line: build outputs: rule inputs | implicit_deps || order_deps
			buildLine := strings.TrimSpace(line[6:]) // Remove "build "

			// Split by colon to separate outputs and rest
			colonParts := strings.SplitN(buildLine, ":", 2)
			if len(colonParts) != 2 {
				continue // Skip invalid build lines
			}

			outputs := p.parseFilePaths(colonParts[0])
			rest := strings.TrimSpace(colonParts[1])

			// Parse rule and dependencies
			parts := strings.Fields(rest)
			if len(parts) == 0 {
				continue // Skip if no rule specified
			}

			rule := parts[0]
			var inputs, implicitDeps, orderDeps []string

			// Join remaining parts and split by dependency separators
			if len(parts) > 1 {
				depString := strings.Join(parts[1:], " ")

				// Split by || for order dependencies
				orderParts := strings.Split(depString, "||")
				if len(orderParts) > 1 {
					orderDeps = p.parseFilePaths(strings.TrimSpace(orderParts[1]))
					depString = strings.TrimSpace(orderParts[0])
				}

				// Split by | for implicit dependencies
				implicitParts := strings.Split(depString, "|")
				if len(implicitParts) > 1 {
					implicitDeps = p.parseFilePaths(strings.TrimSpace(implicitParts[1]))
					depString = strings.TrimSpace(implicitParts[0])
				}

				// Remaining are regular inputs
				if depString != "" {
					inputs = p.parseFilePaths(depString)
				}
			}

			currentBuild = &ParsedBuild{
				Rule:         rule,
				Outputs:      outputs,
				Inputs:       inputs,
				ImplicitDeps: implicitDeps,
				OrderDeps:    orderDeps,
				Variables:    make(map[string]string),
				Pool:         "default", // Default pool
			}
			continue
		}

		// Handle other constructs (pools, variables, etc.) - must come before indented line parsing
		if strings.HasPrefix(line, "pool ") || strings.HasPrefix(line, "variable ") {
			// Save current rule if we're switching contexts
			if currentRule != nil {
				if currentRule.Command == "" {
					return fmt.Errorf("rule %s is missing required command", currentRule.Name)
				}
				if _, err := p.store.AddRule(currentRule); err != nil {
					return fmt.Errorf("failed to add rule %s: %w", currentRule.Name, err)
				}
				currentRule = nil
			}

			// Save current build if we're switching contexts
			if currentBuild != nil {
				if err := p.saveBuild(currentBuild); err != nil {
					return fmt.Errorf("failed to save build: %w", err)
				}
				currentBuild = nil
			}
			// Skip pools and variables for now - could be implemented later
			continue
		}

		// Check if this is an indented line
		originalLine := lines[i] // Get the original line to check indentation
		if strings.HasPrefix(originalLine, "  ") || strings.HasPrefix(originalLine, "\t") {
			// Parse rule properties (indented lines after rule declaration)
			if currentRule != nil {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])

					switch key {
					case "command":
						currentRule.Command = value
					case "description":
						currentRule.Description = value
					default:
						// Handle custom variables
						vars, _ := currentRule.GetVariables()
						if vars == nil {
							vars = make(map[string]string)
						}
						vars[key] = value
						_ = currentRule.SetVariables(vars)
					}
				}
				continue
			}

			// Parse build variables (indented lines after build statement)
			if currentBuild != nil {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])

					if key == "pool" {
						currentBuild.Pool = value
					} else {
						currentBuild.Variables[key] = value
					}
				}
				continue
			}
		}
	}

	// Save any remaining rule or build
	if currentRule != nil {
		if currentRule.Command == "" {
			return fmt.Errorf("rule %s is missing required command", currentRule.Name)
		}
		if _, err := p.store.AddRule(currentRule); err != nil {
			return fmt.Errorf("failed to add final rule %s: %w", currentRule.Name, err)
		}
	}

	if currentBuild != nil {
		if err := p.saveBuild(currentBuild); err != nil {
			return fmt.Errorf("failed to save final build: %w", err)
		}
	}

	return nil
}

// saveBuild converts ParsedBuild to store.NinjaBuild and saves it
func (p *NinjaParser) saveBuild(pb *ParsedBuild) error {
	if len(pb.Outputs) == 0 {
		return fmt.Errorf("build must have at least one output")
	}

	// Generate a unique build ID based on outputs
	buildID := strings.Join(pb.Outputs, ",")

	build := &store.NinjaBuild{
		BuildID: buildID,
		Rule:    quad.IRI(fmt.Sprintf("rule:%s", pb.Rule)),
		Pool:    pb.Pool,
	}

	if err := build.SetVariables(pb.Variables); err != nil {
		return fmt.Errorf("failed to set build variables: %w", err)
	}

	return p.store.AddBuild(build, pb.Inputs, pb.Outputs, pb.ImplicitDeps, pb.OrderDeps)
}

// parseFilePaths parses space-separated file paths, handling escaped spaces
func (p *NinjaParser) parseFilePaths(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	var paths []string
	parts := strings.Fields(input)

	for _, part := range parts {
		// Handle escaped spaces and other characters
		part = strings.ReplaceAll(part, `\ `, " ")
		if part != "" {
			paths = append(paths, part)
		}
	}

	return paths
}
