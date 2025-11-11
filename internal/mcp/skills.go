package mcp

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/mcp"
)

var (
	//go:embed specs/local-container-spec.md
	localContainerSpec string

	//go:embed specs/nomad-spec.md
	nomadSpec string
)

type SkillInfo struct {
	Filename    string `json:"filename"`
	Description string `json:"description"`
}

func skills(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	cfg := config.GetServerConfig()
	filename := req.StringOr("filename", "")

	if filename == "" {
		return listSkills(cfg.SkillsPath)
	}
	return getSkillContent(cfg.SkillsPath, filename)
}

func listSkills(skillsPath string) (*mcp.ToolResponse, error) {
	var skills []SkillInfo
	var message string

	// Add user skills if directory exists and is configured
	if skillsPath != "" {
		if _, err := os.Stat(skillsPath); err == nil {
			userSkills, err := scanUserSkills(skillsPath)
			if err != nil {
				return nil, fmt.Errorf("error scanning skills directory: %v", err)
			}
			skills = append(skills, userSkills...)
		}
	} else {
		message = "Skills path not configured - showing built-in specs only"
	}

	// Always add internal specs (but skip if user file with same name exists)
	skills = addInternalSpecs(skills)

	result := map[string]interface{}{
		"action": "list",
		"count":  len(skills),
		"skills": skills,
	}

	if message != "" {
		result["message"] = message
	}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}

func getSkillContent(skillsPath, filename string) (*mcp.ToolResponse, error) {
	var content string
	var err error

	// Try to read from user skills directory first (if configured and exists)
	if skillsPath != "" {
		if _, statErr := os.Stat(skillsPath); statErr == nil {
			content, err = readUserSkill(skillsPath, filename)
			if err == nil {
				// Successfully read user skill
				return createContentResponse(filename, content), nil
			}
			// If file not found, continue to check internal specs
			if !os.IsNotExist(err) {
				// Other error (permission, etc.)
				return nil, err
			}
		}
	}

	// Fallback to internal specs
	if internalContent := getInternalSpec(filename); internalContent != "" {
		return createContentResponse(filename, internalContent), nil
	}

	// File not found anywhere
	if skillsPath == "" {
		return nil, fmt.Errorf("skill file not found: %s (skills path not configured). Available built-in specs: nomad-spec.md, local-container-spec.md. Use skills() without filename to list all available skills", filename)
	}
	return nil, fmt.Errorf("skill file not found: %s. Use skills() without filename to list all available skills", filename)
}

func scanUserSkills(skillsPath string) ([]SkillInfo, error) {
	var skills []SkillInfo

	err := filepath.Walk(skillsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Only process common text file extensions
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".txt" && ext != ".rst" && ext != ".adoc" && ext != "" {
			return nil
		}

		// Get relative path from skills directory
		relPath, err := filepath.Rel(skillsPath, path)
		if err != nil {
			return err
		}

		// Extract description
		description := extractDescription(path)
		if description == "" {
			description = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}

		skills = append(skills, SkillInfo{
			Filename:    relPath,
			Description: description,
		})

		return nil
	})

	return skills, err
}

func readUserSkill(skillsPath, filename string) (string, error) {
	// Security checks
	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("filename must be relative to skills directory")
	}

	cleanPath := filepath.Clean(filename)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("invalid filename: directory traversal not allowed")
	}

	fullPath := filepath.Join(skillsPath, cleanPath)

	// Ensure the resolved path is still within the skills directory
	absSkillsPath, err := filepath.Abs(skillsPath)
	if err != nil {
		return "", fmt.Errorf("error resolving skills path: %v", err)
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("error resolving file path: %v", err)
	}

	if !strings.HasPrefix(absFullPath, absSkillsPath) {
		return "", fmt.Errorf("file path outside skills directory")
	}

	// Read file content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func createContentResponse(filename, content string) *mcp.ToolResponse {
	result := map[string]interface{}{
		"filename": filename,
		"content":  content,
	}
	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	)
}

func getInternalSpec(filename string) string {
	switch filename {
	case "nomad-spec.md":
		return nomadSpec
	case "local-container-spec.md", "docker-spec.md", "podman-spec.md", "apple-spec.md":
		return localContainerSpec
	default:
		return ""
	}
}

func addInternalSpecs(skills []SkillInfo) []SkillInfo {
	internalSpecs := map[string]string{
		"nomad-spec.md":           nomadSpec,
		"local-container-spec.md": localContainerSpec,
	}

	for filename, content := range internalSpecs {
		// Check if this spec is already in the list (user override)
		found := false
		for _, skill := range skills {
			if skill.Filename == filename {
				found = true
				break
			}
		}

		// If not found, add the internal spec
		if !found {
			title := extractTitleFromContent(content)
			if title == "" {
				title = strings.TrimSuffix(filename, filepath.Ext(filename))
			}

			skills = append(skills, SkillInfo{
				Filename:    filename,
				Description: title,
			})
		}
	}

	return skills
}

func extractDescription(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	return extractTitleFromReader(file)
}

func extractTitleFromContent(content string) string {
	return extractTitleFromReader(strings.NewReader(content))
}

func extractTitleFromReader(reader interface{ Read([]byte) (int, error) }) string {
	scanner := bufio.NewScanner(reader)
	lineCount := 0

	// Look for front matter first (YAML or TOML)
	if scanner.Scan() {
		firstLine := strings.TrimSpace(scanner.Text())
		if firstLine == "---" || firstLine == "+++" {
			// Parse front matter
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "---" || line == "+++" {
					break
				}

				// Look for title field first, then description as fallback
				if strings.HasPrefix(strings.ToLower(line), "title:") {
					title := strings.TrimSpace(strings.TrimPrefix(line, "title:"))
					title = strings.Trim(title, `"'`)
					if title != "" {
						return title
					}
				} else if strings.HasPrefix(strings.ToLower(line), "description:") {
					desc := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
					desc = strings.Trim(desc, `"'`)
					if desc != "" {
						return desc
					}
				}
			}
		} else {
			// Reset and look for headings
			lineCount = 1
		}
	}

	// Look for first heading or meaningful line
	for scanner.Scan() && lineCount < 10 {
		line := strings.TrimSpace(scanner.Text())
		lineCount++

		if line == "" {
			continue
		}

		// Markdown heading
		if strings.HasPrefix(line, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if heading != "" {
				return heading
			}
		}

		// First non-empty line as fallback (but skip common prefixes)
		if !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "/*") &&
			!strings.HasPrefix(line, "<!--") && len(line) > 10 {
			// Remove common markdown/markup
			cleaned := regexp.MustCompile(`[*_`+"`"+`]+`).ReplaceAllString(line, "")
			cleaned = strings.TrimSpace(cleaned)
			if len(cleaned) > 5 && len(cleaned) < 100 {
				return cleaned
			}
		}
	}

	return ""
}

// GetInternalNomadSpec returns the embedded nomad spec (for scaffold command)
func GetInternalNomadSpec() string {
	return nomadSpec
}

// GetInternalLocalContainerSpec returns the embedded local container spec (for scaffold command)
func GetInternalLocalContainerSpec() string {
	return localContainerSpec
}

// Deprecated: Use GetInternalLocalContainerSpec instead
func GetInternalDockerSpec() string {
	return localContainerSpec
}

// Deprecated: Use GetInternalLocalContainerSpec instead
func GetInternalPodmanSpec() string {
	return localContainerSpec
}

// Deprecated: Use GetInternalLocalContainerSpec instead
func GetInternalAppleSpec() string {
	return localContainerSpec
}
