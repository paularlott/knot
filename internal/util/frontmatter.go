package util

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

type SkillFrontmatter struct {
	Name        string
	Description string
}

type CommandFrontmatter struct {
	Name         string
	Description  string
	ArgumentHint string
	AllowedTools []string
}

var skillNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var commandNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func ParseSkillFrontmatter(content string) (*SkillFrontmatter, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty content")
	}

	firstLine := strings.TrimSpace(scanner.Text())
	if firstLine != "---" && firstLine != "+++" {
		return nil, fmt.Errorf("frontmatter not found: must start with --- or +++")
	}

	delimiter := firstLine
	fm := &SkillFrontmatter{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == delimiter {
			break
		}

		if strings.HasPrefix(strings.ToLower(line), "name:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.TrimPrefix(name, "Name:")
			name = strings.Trim(name, `"'`)
			fm.Name = name
		} else if strings.HasPrefix(strings.ToLower(line), "description:") {
			desc := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			desc = strings.TrimPrefix(desc, "Description:")
			desc = strings.Trim(desc, `"'`)
			fm.Description = desc
		}
	}

	if fm.Name == "" {
		return nil, fmt.Errorf("name field is required in frontmatter")
	}
	if fm.Description == "" {
		return nil, fmt.Errorf("description field is required in frontmatter")
	}

	if !skillNameRegex.MatchString(fm.Name) {
		return nil, fmt.Errorf("invalid name: must be 1-64 lowercase letters, numbers, hyphens only, starting with a letter")
	}

	if len(fm.Description) > 1024 {
		return nil, fmt.Errorf("description exceeds 1024 characters")
	}

	return fm, nil
}

func ParseCommandFrontmatter(content string) (*CommandFrontmatter, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	if !scanner.Scan() {
		return nil, fmt.Errorf("empty content")
	}

	firstLine := strings.TrimSpace(scanner.Text())
	if firstLine != "---" && firstLine != "+++" {
		return nil, fmt.Errorf("frontmatter not found: must start with --- or +++")
	}

	delimiter := firstLine
	fm := &CommandFrontmatter{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == delimiter {
			break
		}

		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "name:"):
			name := strings.TrimSpace(strings.TrimPrefix(line, line[:strings.Index(line, ":")+1]))
			name = strings.Trim(name, `"'`)
			fm.Name = name
		case strings.HasPrefix(lower, "description:"):
			desc := strings.TrimSpace(strings.TrimPrefix(line, line[:strings.Index(line, ":")+1]))
			desc = strings.Trim(desc, `"'`)
			fm.Description = desc
		case strings.HasPrefix(lower, "argument-hint:") || strings.HasPrefix(lower, "argument_hint:"):
			hint := strings.TrimSpace(strings.TrimPrefix(line, line[:strings.Index(line, ":")+1]))
			hint = strings.Trim(hint, `"'`)
			fm.ArgumentHint = hint
		case strings.HasPrefix(lower, "allowed-tools:") || strings.HasPrefix(lower, "allowed_tools:"):
			tools := strings.TrimSpace(strings.TrimPrefix(line, line[:strings.Index(line, ":")+1]))
			tools = strings.Trim(tools, `"'`)
			if tools != "" {
				for _, t := range strings.Split(tools, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						fm.AllowedTools = append(fm.AllowedTools, t)
					}
				}
			}
		}
	}

	if fm.Name == "" {
		return nil, fmt.Errorf("name field is required in frontmatter")
	}
	if fm.Description == "" {
		return nil, fmt.Errorf("description field is required in frontmatter")
	}

	if !commandNameRegex.MatchString(fm.Name) {
		return nil, fmt.Errorf("invalid name: must be 1-64 lowercase letters, numbers, hyphens only, starting with a letter")
	}

	if len(fm.Description) > 1024 {
		return nil, fmt.Errorf("description exceeds 1024 characters")
	}

	return fm, nil
}
