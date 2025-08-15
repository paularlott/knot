package mcp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/mcp"
)

type RecipeInfo struct {
	Filename    string `json:"filename"`
	Description string `json:"description"`
}

func recipes(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	cfg := config.GetServerConfig()

	if cfg.RecipesPath == "" {
		// Return empty list when recipes path is not configured
		filename := req.StringOr("filename", "")
		if filename == "" {
			// Return empty recipe list
			result := map[string]interface{}{
				"action":  "list",
				"recipes": []RecipeInfo{},
				"count":   0,
				"message": "Recipes path not configured",
			}
			return mcp.NewToolResponseJSON(result), nil
		} else {
			// Return error for specific file requests when path not configured
			return nil, fmt.Errorf("Recipes path not configured - cannot retrieve specific recipe")
		}
	}

	// Check if recipes directory exists
	if _, err := os.Stat(cfg.RecipesPath); os.IsNotExist(err) {
		filename := req.StringOr("filename", "")
		if filename == "" {
			// Return empty list when directory doesn't exist
			result := map[string]interface{}{
				"action":  "list",
				"recipes": []RecipeInfo{},
				"count":   0,
				"message": fmt.Sprintf("Recipes directory does not exist: %s", cfg.RecipesPath),
			}
			return mcp.NewToolResponseJSON(result), nil
		} else {
			return nil, fmt.Errorf("Recipes directory does not exist: %s", cfg.RecipesPath)
		}
	}

	filename := req.StringOr("filename", "")

	if filename == "" {
		// List all recipes with descriptions
		return listRecipes(cfg.RecipesPath)
	} else {
		// Get specific recipe content
		return getRecipeContent(cfg.RecipesPath, filename)
	}
}

func listRecipes(recipesPath string) (*mcp.ToolResponse, error) {
	var recipes []RecipeInfo

	err := filepath.Walk(recipesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-text files
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
			// Fallback to filename without extension
			description = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}

		recipes = append(recipes, RecipeInfo{
			Filename:    relPath,
			Description: description,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("Error scanning recipes directory: %v", err)
	}

	result := map[string]interface{}{
		"action":  "list",
		"recipes": recipes,
		"count":   len(recipes),
	}

	return mcp.NewToolResponseJSON(result), nil
}

func getRecipeContent(recipesPath, filename string) (*mcp.ToolResponse, error) {
	// Ensure filename is relative and doesn't escape the recipes directory
	if filepath.IsAbs(filename) {
		return nil, fmt.Errorf("Filename must be relative to recipes directory")
	}

	// Clean the path to prevent directory traversal
	cleanPath := filepath.Clean(filename)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("Invalid filename: directory traversal not allowed")
	}

	fullPath := filepath.Join(recipesPath, cleanPath)

	// Ensure the resolved path is still within the recipes directory
	absRecipesPath, err := filepath.Abs(recipesPath)
	if err != nil {
		return nil, fmt.Errorf("Error resolving recipes path: %v", err)
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, fmt.Errorf("Error resolving file path: %v", err)
	}

	if !strings.HasPrefix(absFullPath, absRecipesPath) {
		return nil, fmt.Errorf("File path outside recipes directory")
	}

	// Read file content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Recipe file not found: %s", filename)
		}
		return nil, fmt.Errorf("Error reading recipe file: %v", err)
	}

	result := map[string]interface{}{
		"action":   "get",
		"filename": filename,
		"content":  string(content),
		"size":     len(content),
	}

	return mcp.NewToolResponseJSON(result), nil
}

func extractDescription(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
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

				// Look for description field
				if strings.HasPrefix(strings.ToLower(line), "description:") {
					desc := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
					desc = strings.Trim(desc, `"'`)
					if desc != "" {
						return desc
					}
				}
			}
		} else {
			// Reset scanner to beginning if no front matter
			file.Seek(0, 0)
			scanner = bufio.NewScanner(file)
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

		// RST heading (underlined)
		if lineCount > 1 && (strings.HasPrefix(line, "===") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "~~~")) {
			// Previous line was likely the heading
			file.Seek(0, 0)
			scanner = bufio.NewScanner(file)
			prevLine := ""
			for i := 0; i < lineCount-1 && scanner.Scan(); i++ {
				prevLine = strings.TrimSpace(scanner.Text())
			}
			if prevLine != "" {
				return prevLine
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
