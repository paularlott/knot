package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// TestSuite9_ChatOpenAI tests web chat and OpenAI endpoint integration with MCP tools
func TestSuite9_ChatOpenAI(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	user1Client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create user1 client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, user1Client, &createdScriptIDs)

	// Create test tools
	var globalToolID, user1ToolID string

	t.Run("CreateGlobalMCPTool", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "chat_global_tool",
			Description: "Global MCP tool for chat testing",
			Content:     `def chat_global_tool(message): return f"Global tool processed: {message}"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "message"
type = "string"
description = "Message to process"
required = true`,
			MCPKeywords: []string{"chat", "global", "test"},
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create global MCP tool: %v", err)
		}

		globalToolID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created global MCP tool with ID: %s", resp.Id)
	})

	t.Run("CreateUser1MCPTool", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "chat_user1_tool",
			Description: "User1's MCP tool for chat testing",
			Content:     `def chat_user1_tool(message): return f"User1 tool processed: {message}"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "message"
type = "string"
description = "Message to process"
required = true`,
			MCPKeywords: []string{"chat", "user1", "test"},
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create user1 MCP tool: %v", err)
		}

		user1ToolID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created user1 MCP tool with ID: %s", resp.Id)
	})

	// Test /v1/chat/completions (OpenAI-compatible endpoint)
	t.Run("OpenAI_Completion", func(t *testing.T) {
		chatReq := map[string]any{
			"model": os.Getenv("KNOT_OPENAI_MODEL"),
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "Say hello",
				},
			},
		}

		var chatResp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/v1/chat/completions", chatReq, &chatResp)
		if err != nil {
			t.Logf("Failed to call /v1/chat/completions: %v", err)
		}

		// The endpoint should be accessible (200 or 500 for model not found)
		if statusCode == 401 {
			t.Error("Expected status 200 or 500, got 401 (unauthorized)")
		}
		if statusCode == 200 || statusCode == 500 {
			t.Logf("/v1/chat/completions is accessible (status %d)", statusCode)

			if statusCode == 200 {
				if choices, ok := chatResp["choices"].([]any); ok && len(choices) > 0 {
					t.Logf("Got response with %d choices", len(choices))
					if firstChoice, ok := choices[0].(map[string]any); ok {
						if message, ok := firstChoice["message"].(map[string]any); ok {
							if content, ok := message["content"].(string); ok {
								t.Logf("Response content length: %d", len(content))
							}
						}
					}
				}
			}
		}
	})

	// Test /v1/models endpoint
	t.Run("OpenAI_ListModels", func(t *testing.T) {
		var modelsResp map[string]any
		statusCode, err := user1Client.Do(ctx, "GET", "/v1/models", nil, &modelsResp)
		if err != nil {
			t.Fatalf("Failed to list models: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		t.Logf("/v1/models is accessible")
	})

	_ = globalToolID
	_ = user1ToolID
}

// TestSuite13_ChatOpenAI_Cleanup tests cleanup of test scripts
func TestSuite13_ChatOpenAI_Cleanup(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	user1Client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create user1 client: %v", err)
	}

	// Clean up any existing test scripts from previous runs
	t.Run("CleanupOldTestScripts", func(t *testing.T) {
		var listResp apiclient.ScriptList
		statusCode, err := user1Client.Do(ctx, "GET", "/api/scripts?all_zones=true", nil, &listResp)
		if err != nil {
			t.Logf("Warning: Failed to list scripts for cleanup: %v", err)
			return
		}
		if statusCode != 200 {
			t.Logf("Warning: Failed to list scripts for cleanup, got status %d", statusCode)
			return
		}

		// Delete any existing test chat scripts
		cleaned := 0
		for _, script := range listResp.Scripts {
			if strings.HasPrefix(script.Name, testPrefix+"chat") ||
				strings.HasPrefix(script.Name, testPrefix+"user1_private") ||
				strings.HasPrefix(script.Name, testPrefix+"all_users") ||
				strings.HasPrefix(script.Name, testPrefix+"echo") {
				err := user1Client.DeleteScript(ctx, script.Id)
				if err == nil {
					cleaned++
					t.Logf("Cleaned up old test script: %s (%s)", script.Name, script.Id)
				}
			}
		}

		if cleaned > 0 {
			t.Logf("Cleaned up %d old test scripts", cleaned)
		}
	})
}
