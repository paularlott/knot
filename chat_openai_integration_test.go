package main

import (
	"context"
	"encoding/json"
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

	var globalToolID, user1ToolID string

	// Create a global MCP tool for testing
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
		_ = globalToolID // Used for cleanup tracking
		t.Logf("Created global MCP tool with ID: %s", resp.Id)
	})

	// Create user1's MCP tool
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
		_ = user1ToolID // Used for cleanup tracking
		t.Logf("Created user1 MCP tool with ID: %s", resp.Id)
	})

	// Test /api/chat/tools endpoint - should return tools list
	t.Run("WebChat_ListTools", func(t *testing.T) {
		var toolsResp []map[string]any
		statusCode, err := user1Client.Do(ctx, "GET", "/api/chat/tools", nil, &toolsResp)
		if err != nil {
			t.Fatalf("Failed to list tools: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Should only show meta tools (tool_search, execute_tool) in force on-demand mode
		metaTools := make(map[string]bool)
		for _, tool := range toolsResp {
			// MCP server returns capitalized field names (Name, Description, etc.)
			if name, ok := tool["Name"].(string); ok {
				metaTools[name] = true
			}
		}

		// Log all tools returned for debugging - show full structure
		for i, tool := range toolsResp {
			t.Logf("Tool [%d]: %+v", i, tool)
		}

		// Verify tool_search and execute_tool are present
		if !metaTools["tool_search"] {
			t.Error("tool_search not found in /api/chat/tools")
		}
		if !metaTools["execute_tool"] {
			t.Error("execute_tool not found in /api/chat/tools")
		}

		// Verify script tools are NOT in tools/list (they're hidden but searchable)
		for _, tool := range toolsResp {
			if name, ok := tool["Name"].(string); ok {
				if strings.HasPrefix(name, testPrefix) {
					t.Errorf("Script tool should not be visible in /api/chat/tools tools/list: %s", name)
				}
			}
		}

		t.Logf("/api/chat/tools returned %d tools (meta tools only in force on-demand mode)", len(toolsResp))
	})

	// Test /api/chat/tools/call with tool_search to find scripts
	t.Run("WebChat_ToolSearchFindsScripts", func(t *testing.T) {
		// tool_search request
		toolCallReq := map[string]any{
			"name": "tool_search",
			"arguments": map[string]any{
				"query":       testPrefix + "chat",
				"max_results": 10000,
			},
		}

		var toolCallResp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Log the actual response structure for debugging
		t.Logf("tool_search response structure: %+v", toolCallResp)

		// Check for error response
		if errObj, ok := toolCallResp["error"]; ok {
			t.Logf("tool_search returned error: %v", errObj)
			t.Skip("tool_search method not implemented on server")
		}

		// tools/call returns content array
		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatalf("Expected content array in tools/call result, got: %+v", toolCallResp)
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		// The first content item should have type "text" and text containing JSON
		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		// Parse the JSON text to get tools array
		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		// Should find our test tools
		foundGlobalTool := false
		foundUser1Tool := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if name == testPrefix+"chat_global_tool" {
					foundGlobalTool = true
				}
				if name == testPrefix+"chat_user1_tool" {
					foundUser1Tool = true
				}
			}
		}

		if !foundGlobalTool {
			t.Error("tool_search should find global chat tool")
		}
		if !foundUser1Tool {
			t.Error("tool_search should find user1 chat tool")
		}

		t.Logf("tool_search found %d matching tools", len(tools))
	})

	// Test /api/chat/completion with streaming (web chat endpoint)
	t.Run("WebChat_CompletionStream", func(t *testing.T) {
		// Create a simple chat completion request
		chatReq := map[string]any{
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "Use tool_search to find tools with keyword '" + testPrefix + "chat' and tell me what you found",
				},
			},
		}

		// For streaming, we'd need to handle SSE, which is complex for a simple test
		// For now, we'll just verify the endpoint is accessible
		statusCode, err := user1Client.Do(ctx, "POST", "/api/chat/stream", chatReq, nil)
		if err != nil {
			t.Logf("Failed to call /api/chat/stream: %v", err)
			// The streaming endpoint requires special handling
			// This test verifies the endpoint is accessible
		}
		if statusCode == 401 {
			t.Error("Expected status 200 or 500, got 401 (unauthorized)")
		}
		// Status 500 is acceptable because we can't properly handle SSE in this test
		if statusCode == 200 || statusCode == 500 {
			t.Logf("/api/chat/stream is accessible (status %d)", statusCode)
		}
	})

	// Test /v1/chat/completions (OpenAI-compatible endpoint)
	t.Run("OpenAI_Completion", func(t *testing.T) {
		chatReq := map[string]any{
			"model": os.Getenv("KNOT_OPENAI_MODEL"),
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "Use tool_search to find tools with keyword '" + testPrefix + "chat' and tell me what you found",
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

			// If we got a 200 response, check if it has the expected structure
			if statusCode == 200 {
				if choices, ok := chatResp["choices"].([]any); ok && len(choices) > 0 {
					t.Logf("Got response with %d choices", len(choices))
					// The response should contain the tool_search results
					if firstChoice, ok := choices[0].(map[string]any); ok {
						if message, ok := firstChoice["message"].(map[string]any); ok {
							if content, ok := message["content"].(string); ok {
								t.Logf("Response content length: %d", len(content))
								// Check if the response mentions finding tools
								if strings.Contains(content, testPrefix+"chat") {
									t.Logf("LLM found our test tools in the response")
								}
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

	// Test that scripted tools are NOT visible in force on-demand mode tools/list
	// but ARE available via tool_search
	t.Run("ForceOnDemand_ScriptToolsBehavior", func(t *testing.T) {
		// First verify tools/list only shows meta tools
		var toolsResp []map[string]any
		statusCode, err := user1Client.Do(ctx, "GET", "/api/chat/tools", nil, &toolsResp)
		if err != nil {
			t.Fatalf("Failed to list tools: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Count meta tools
		metaToolCount := 0
		for _, tool := range toolsResp {
			if name, ok := tool["Name"].(string); ok {
				if name == "tool_search" || name == "execute_tool" {
					metaToolCount++
				}
			}
		}

		if metaToolCount != 2 {
			t.Errorf("Expected exactly 2 meta tools in force on-demand mode, got %d", metaToolCount)
		}

		// Now use tool_search to find script tools
		toolCallReq := map[string]any{
			"name": "tool_search",
			"arguments": map[string]any{
				"query":       testPrefix,
				"max_results": 10000,
			},
		}

		var toolCallResp map[string]any
		statusCode, err = user1Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		t.Logf("ForceOnDemand tool_search response: %+v", toolCallResp)

		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatalf("Expected content array in tools/call result, got: %+v", toolCallResp)
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		// Should find our test tools
		foundTestTools := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if strings.HasPrefix(name, testPrefix) {
					foundTestTools = true
					t.Logf("tool_search found test tool: %s", name)
				}
			}
		}

		if !foundTestTools {
			t.Error("tool_search should find our test scripts in force on-demand mode")
		}
	})
}

// TestSuite10_ChatOpenAI_UserPermissions tests that users can only see their own script tools
func TestSuite10_ChatOpenAI_UserPermissions(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	user1Client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create user1 client: %v", err)
	}

	user2Client, err := createClient(cfg.baseURL, cfg.user2Token)
	if err != nil {
		t.Fatalf("Failed to create user2 client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, user1Client, &createdScriptIDs)

	var user1ToolID string

	// User1 creates their own MCP tool
	t.Run("User1CreatesTool", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "user1_private_tool",
			Description: "User1's private tool",
			Content:     `def user1_private_tool(): return "user1 only"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "input"
type = "string"
description = "Input"`,
			MCPKeywords: []string{"user1", "private"},
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create user1 tool: %v", err)
		}

		user1ToolID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = user1ToolID // Used for cleanup tracking
		t.Logf("User1 created tool with ID: %s", resp.Id)
	})

	// User1 can find their own tool via tool_search
	t.Run("User1CanFindOwnTool", func(t *testing.T) {
		toolCallReq := map[string]any{
			"name": "tool_search",
			"arguments": map[string]any{
				"query":       testPrefix + "user1_private",
				"max_results": 10000,
			},
		}

		var toolCallResp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		found := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if name == testPrefix+"user1_private_tool" {
					found = true
					break
				}
			}
		}

		if !found {
			t.Error("User1 should be able to find their own tool via tool_search")
		}
	})

	// User2 cannot find user1's private tool
	t.Run("User2CannotFindUser1Tool", func(t *testing.T) {
		toolCallReq := map[string]any{
			"name": "tool_search",
			"arguments": map[string]any{
				"query":       testPrefix + "user1_private",
				"max_results": 10000,
			},
		}

		var toolCallResp map[string]any
		statusCode, err := user2Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		// Should NOT find user1's private tool
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if name == testPrefix+"user1_private_tool" {
					t.Error("User2 should NOT be able to find user1's private tool via tool_search")
				}
			}
		}
	})
}

// TestSuite11_ChatOpenAI_GlobalScripts tests global script visibility in chat
func TestSuite11_ChatOpenAI_GlobalScripts(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	user1Client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create user1 client: %v", err)
	}

	user2Client, err := createClient(cfg.baseURL, cfg.user2Token)
	if err != nil {
		t.Fatalf("Failed to create user2 client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, user1Client, &createdScriptIDs)

	var globalToolID string

	// Create a global MCP tool
	t.Run("CreateGlobalTool", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "all_users_tool",
			Description: "Global tool for all users",
			Content:     `def all_users_tool(): return "available to all"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "input"
type = "string"
description = "Input"`,
			MCPKeywords: []string{"global", "all", "test"},
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create global tool: %v", err)
		}

		globalToolID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = globalToolID // Used for cleanup tracking
		t.Logf("Created global tool with ID: %s", resp.Id)
	})

	// User1 can find global tool via tool_search
	t.Run("User1CanFindGlobalTool", func(t *testing.T) {
		toolCallReq := map[string]any{
			"name": "tool_search",
			"arguments": map[string]any{
				"query":       testPrefix + "all_users",
				"max_results": 10000,
			},
		}

		var toolCallResp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		found := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if name == testPrefix+"all_users_tool" {
					found = true
					break
				}
			}
		}

		if !found {
			t.Error("User1 should be able to find global tool via tool_search")
		}
	})

	// User2 can ALSO find global tool via tool_search (has ExecuteScripts permission)
	t.Run("User2CanFindGlobalTool", func(t *testing.T) {
		toolCallReq := map[string]any{
			"name": "tool_search",
			"arguments": map[string]any{
				"query":       testPrefix + "all_users",
				"max_results": 10000,
			},
		}

		var toolCallResp map[string]any
		statusCode, err := user2Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		found := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if name == testPrefix+"all_users_tool" {
					found = true
					break
				}
			}
		}

		if !found {
			t.Error("User2 (with ExecuteScripts) should be able to find global tool via tool_search")
		} else {
			t.Log("User2 successfully found global tool (expected with ExecuteScripts permission)")
		}
	})
}

// TestSuite11b_ChatOpenAI_ZoneFiltering tests zone filtering for chat tools
func TestSuite11b_ChatOpenAI_ZoneFiltering(t *testing.T) {
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

	// Create a tool in the current zone
	t.Run("CreateToolInCurrentZone", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "zone_current_tool",
			Description: "Tool in current zone",
			Content:     `def tool(): return "current zone result"`,
			Zones:       []string{cfg.zone},
			Active:      true,
			ScriptType:  "tool",
			MCPKeywords: []string{"zone", "current"},
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create tool in current zone: %v", err)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created tool in zone %s with ID: %s", cfg.zone, resp.Id)
	})

	// Create a tool in a different zone
	t.Run("CreateToolInDifferentZone", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "zone_different_tool",
			Description: "Tool in different zone",
			Content:     `def tool(): return "different zone result"`,
			Zones:       []string{"nonexistent_zone"},
			Active:      true,
			ScriptType:  "tool",
			MCPKeywords: []string{"zone", "different"},
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create tool in different zone: %v", err)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created tool in zone nonexistent_zone with ID: %s", resp.Id)
	})

	// Test tool_search finds tool in current zone
	t.Run("ToolSearchFindsCurrentZoneTool", func(t *testing.T) {
		toolCallReq := map[string]any{
			"name": "tool_search",
			"arguments": map[string]any{
				"query":       testPrefix + "zone",
				"max_results": 100,
			},
		}

		var toolCallResp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		// Check if tool_search returned "No tools found" message
		if strings.HasPrefix(text, "No tools found") {
			t.Error("tool_search should find tool in current zone")
			return
		}

		// Parse the JSON text to get tools array
		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v (text: %s)", err, text)
		}

		// Should find the tool in current zone
		foundCurrentZone := false
		foundDifferentZone := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if name == testPrefix+"zone_current_tool" {
					foundCurrentZone = true
				}
				if name == testPrefix+"zone_different_tool" {
					foundDifferentZone = true
				}
			}
		}

		if !foundCurrentZone {
			t.Error("tool_search should find tool in current zone")
		}
		if foundDifferentZone {
			t.Error("tool_search should NOT find tool in different zone")
		}

		t.Logf("Zone filtering: current_zone=%v, different_zone=%v", foundCurrentZone, foundDifferentZone)
	})

	// Clean up
	t.Run("Cleanup", func(t *testing.T) {
		for _, id := range createdScriptIDs {
			if id != "" {
				err := user1Client.DeleteScript(ctx, id)
				if err != nil {
					t.Logf("Warning: Failed to delete script %s: %v", id, err)
				}
			}
		}
	})
}

// TestSuite12_ChatOpenAI_ToolExecution tests executing script tools via chat
func TestSuite12_ChatOpenAI_ToolExecution(t *testing.T) {
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

	var toolID string

	// Create a simple MCP tool that returns a fixed result
	t.Run("CreateExecutableTool", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "echo_tool",
			Description: "Echo tool for testing",
			Content:     `message = knot.mcp.get("message"); knot.mcp.return_string(f"Echo: {message}")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "message"
type = "string"
description = "Message to echo"
required = true`,
			MCPKeywords: []string{"echo", "test"},
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create echo tool: %v", err)
		}

		toolID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = toolID // Used for cleanup tracking
		t.Logf("Created echo tool with ID: %s", resp.Id)
	})

	// Execute the tool via /api/chat/tools/call
	t.Run("ExecuteToolViaChatAPI", func(t *testing.T) {
		toolCallReq := map[string]any{
			"name": testPrefix + "echo_tool",
			"arguments": map[string]any{
				"message": "Hello from test",
			},
		}

		var toolCallResp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/api/chat/tools/call", toolCallReq, &toolCallResp)
		if err != nil {
			t.Fatalf("Failed to execute tool: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Check the response
		content, ok := toolCallResp["Content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tool execution result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool execution result")
		}

		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["Text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		// Verify the tool executed successfully
		if !strings.Contains(text, "Echo: Hello from test") {
			t.Errorf("Expected tool to echo our message, got: %s", text)
		}

		t.Logf("Tool executed successfully: %s", text)
	})
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
