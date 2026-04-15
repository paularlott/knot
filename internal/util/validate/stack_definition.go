package validate

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
)

// ValidateStackDefinition checks structural validity of a stack definition request.
// Returns a slice of validation errors (empty if valid).
func ValidateStackDefinition(req *apiclient.StackDefinitionRequest) []apiclient.ValidationError {
	var errors []apiclient.ValidationError

	// R1: Name is required
	if req.Name == "" {
		errors = append(errors, apiclient.ValidationError{
			Field:   "name",
			Message: "Name is required",
		})
	}

	// Build a set of space names for reference checks
	spaceNames := make(map[string]bool, len(req.Spaces))

	for i, space := range req.Spaces {
		// R3: Space must have a name
		if space.Name == "" {
			errors = append(errors, apiclient.ValidationError{
				Field:   fmt.Sprintf("spaces[%d].name", i),
				Message: fmt.Sprintf("Space at index %d must have a name", i),
			})
			continue
		}

		// R2: Duplicate space names
		if spaceNames[space.Name] {
			errors = append(errors, apiclient.ValidationError{
				Field:   fmt.Sprintf("spaces[%d].name", i),
				Message: fmt.Sprintf("Duplicate space name: %s", space.Name),
				Space:   space.Name,
			})
		}
		spaceNames[space.Name] = true

		// R4: Template ID is required
		if space.TemplateId == "" {
			errors = append(errors, apiclient.ValidationError{
				Field:   fmt.Sprintf("spaces[%d].template_id", i),
				Message: fmt.Sprintf("Space %s must have a template_id", space.Name),
				Space:   space.Name,
			})
		}

		// R5: depends_on references must exist
		for _, dep := range space.DependsOn {
			if !spaceNames[dep] {
				// Check if it's a name that appears later (we need to check the full list)
				found := false
				for _, s := range req.Spaces {
					if s.Name == dep {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, apiclient.ValidationError{
						Field:   fmt.Sprintf("spaces[%d].depends_on", i),
						Message: fmt.Sprintf("Space %s depends_on '%s' not found in definition", space.Name, dep),
						Space:   space.Name,
					})
				}
			}
		}

		// R7: port_forwards to_space references must exist
		for j, pf := range space.PortForwards {
			if pf.ToSpace != "" {
				found := false
				for _, s := range req.Spaces {
					if s.Name == pf.ToSpace {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, apiclient.ValidationError{
						Field:   fmt.Sprintf("spaces[%d].port_forwards[%d].to_space", i, j),
						Message: fmt.Sprintf("Space %s port_forward to_space '%s' not found in definition", space.Name, pf.ToSpace),
						Space:   space.Name,
					})
				}
			}

			// R8/R9: Port ranges (uint16 so only need to check for 0)
			if pf.LocalPort == 0 {
				errors = append(errors, apiclient.ValidationError{
					Field:   fmt.Sprintf("spaces[%d].port_forwards[%d].local_port", i, j),
					Message: fmt.Sprintf("Space %s local_port must be 1-65535, got 0", space.Name),
					Space:   space.Name,
				})
			}
			if pf.RemotePort == 0 {
				errors = append(errors, apiclient.ValidationError{
					Field:   fmt.Sprintf("spaces[%d].port_forwards[%d].remote_port", i, j),
					Message: fmt.Sprintf("Space %s remote_port must be 1-65535, got 0", space.Name),
					Space:   space.Name,
				})
			}
		}
	}

	// R6: Circular dependency detection via DFS
	if cycleErrors := detectCycles(req.Spaces); len(cycleErrors) > 0 {
		errors = append(errors, cycleErrors...)
	}

	return errors
}

// detectCycles performs DFS-based cycle detection on space dependencies.
func detectCycles(spaces []apiclient.StackDefSpace) []apiclient.ValidationError {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, s := range spaces {
		graph[s.Name] = s.DependsOn
	}

	var errors []apiclient.ValidationError
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully processed
	)
	color := make(map[string]int)

	var dfs func(name string, path []string)
	dfs = func(name string, path []string) {
		color[name] = gray
		path = append(path, name)

		for _, dep := range graph[name] {
			if color[dep] == gray {
				// Found a cycle - find where it loops back
				errors = append(errors, apiclient.ValidationError{
					Field:   "depends_on",
					Message: fmt.Sprintf("Circular dependency detected: %s -> %s", formatPath(path, dep), dep),
					Space:   name,
				})
			} else if color[dep] == white {
				dfs(dep, path)
			}
		}
		color[name] = black
	}

	// Run DFS from each unvisited node
	for _, s := range spaces {
		if color[s.Name] == white {
			dfs(s.Name, nil)
		}
	}

	return errors
}

// formatPath formats the dependency path for error messages.
func formatPath(path []string, cycleTarget string) string {
	// Find where the cycle starts
	start := 0
	for i, n := range path {
		if n == cycleTarget {
			start = i
			break
		}
	}
	var b strings.Builder
	b.WriteString(path[start])
	for i := start + 1; i < len(path); i++ {
		b.WriteString(" -> ")
		b.WriteString(path[i])
	}
	return b.String()
}
