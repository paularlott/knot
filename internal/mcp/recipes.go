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
	//go:embed specs/docker-spec.md
	dockerSpec string

	//go:embed specs/podman-spec.md
	podmanSpec string

	//go:embed specs/nomad-spec.md
	nomadSpec string

	//go:embed specs/apple-spec.md
	appleSpec string
)

type RecipeInfo struct {
	Filename    string `json:"filename"`
	Description string `json:"description"`
}

func recipes(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	cfg := config.GetServerConfig()
	filename := req.StringOr("filename", "")

	if filename == "" {
		return listRecipes(cfg.RecipesPath)
	}
	return getRecipeContent(cfg.RecipesPath, filename)
}

func listRecipes(recipesPath string) (*mcp.ToolResponse, error) {
	var recipes []RecipeInfo

	// Add user recipes if directory exists and is configured
	if recipesPath != "" {
		if _, err := os.Stat(recipesPath); err == nil {
			userRecipes, err := scanUserRecipes(recipesPath)
			if err != nil {
				return nil, fmt.Errorf("error scanning recipes directory: %v", err)
			}
			recipes = append(recipes, userRecipes...)
		}
	}

	// Always add internal specs (but skip if user file with same name exists)
	recipes = addInternalSpecs(recipes)

	result := map[string]interface{}{
		"recipes": recipes,
	}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}

func getRecipeContent(recipesPath, filename string) (*mcp.ToolResponse, error) {
	var content string
	var err error

	// Try to read from user recipes directory first (if configured and exists)
	if recipesPath != "" {
		if _, statErr := os.Stat(recipesPath); statErr == nil {
			content, err = readUserRecipe(recipesPath, filename)
			if err == nil {
				// Successfully read user recipe
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
	if recipesPath == "" {
		return nil, fmt.Errorf("recipe file not found: %s (recipes path not configured). Available built-in specs: nomad-spec.md, docker-spec.md, podman-spec.md, apple-spec.md. Use recipes() without filename to list all available recipes", filename)
	}
	return nil, fmt.Errorf("recipe file not found: %s. Use recipes() without filename to list all available recipes", filename)
}

func scanUserRecipes(recipesPath string) ([]RecipeInfo, error) {
	var recipes []RecipeInfo

	err := filepath.Walk(recipesPath, func(path string, info os.FileInfo, err error) error {
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

		// Get relative path from recipes directory
		relPath, err := filepath.Rel(recipesPath, path)
		if err != nil {
			return err
		}

		// Extract description
		description := extractDescription(path)
		if description == "" {
			description = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}

		recipes = append(recipes, RecipeInfo{
			Filename:    relPath,
			Description: description,
		})

		return nil
	})

	return recipes, err
}

func readUserRecipe(recipesPath, filename string) (string, error) {
	// Security checks
	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("filename must be relative to recipes directory")
	}

	cleanPath := filepath.Clean(filename)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("invalid filename: directory traversal not allowed")
	}

	fullPath := filepath.Join(recipesPath, cleanPath)

	// Ensure the resolved path is still within the recipes directory
	absRecipesPath, err := filepath.Abs(recipesPath)
	if err != nil {
		return "", fmt.Errorf("error resolving recipes path: %v", err)
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("error resolving file path: %v", err)
	}

	if !strings.HasPrefix(absFullPath, absRecipesPath) {
		return "", fmt.Errorf("file path outside recipes directory")
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
	case "docker-spec.md":
		return dockerSpec
	case "podman-spec.md":
		return podmanSpec
	case "apple-spec.md":
		return appleSpec
	default:
		return ""
	}
}

func addInternalSpecs(recipes []RecipeInfo) []RecipeInfo {
	internalSpecs := map[string]string{
		"nomad-spec.md":  nomadSpec,
		"docker-spec.md": dockerSpec,
		"podman-spec.md": podmanSpec,
		"apple-spec.md":  appleSpec,
	}

	for filename, content := range internalSpecs {
		// Check if this spec is already in the list (user override)
		found := false
		for _, recipe := range recipes {
			if recipe.Filename == filename {
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

			recipes = append(recipes, RecipeInfo{
				Filename:    filename,
				Description: title,
			})
		}
	}

	return recipes
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

// GetInternalDockerSpec returns the embedded docker spec (for scaffold command)
func GetInternalDockerSpec() string {
	return dockerSpec
}

// GetInternalPodmanSpec returns the embedded podman spec (for scaffold command)
func GetInternalPodmanSpec() string {
	return dockerSpec
}

// GetInternalAppleSpec returns the embedded apple spec (for scaffold command)
func GetInternalAppleSpec() string {
	return appleSpec
}
