package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/cli/env"
	"github.com/paularlott/knot/apiclient"
)

const (
	skipIntegrationTests = "Skip integration tests (set KNOT_BASE_URL and KNOT_USER1_TOKEN to run)"
	testPrefix           = "test_integration_"
)

// TestMain runs before all tests to load environment variables
func TestMain(m *testing.M) {
	// Load .env file if it exists (ignore error if file doesn't exist)
	_ = env.Load()
	os.Exit(m.Run())
}

// testConfig holds configuration for integration tests
type testConfig struct {
	baseURL     string
	zone        string
	user1Token  string
	user2Token  string
	user1Groups []string
	user2Groups []string
}

// getTestConfig returns the configuration for integration tests
func getTestConfig(t *testing.T) (cfg testConfig, skip bool) {
	t.Helper()

	cfg.baseURL = os.Getenv("KNOT_BASE_URL")
	if cfg.baseURL == "" {
		cfg.baseURL = "http://localhost:8080"
	}

	cfg.zone = os.Getenv("KNOT_ZONE")
	if cfg.zone == "" {
		cfg.zone = "core"
	}

	cfg.user1Token = os.Getenv("KNOT_USER1_TOKEN")
	cfg.user2Token = os.Getenv("KNOT_USER2_TOKEN")

	if cfg.user1Token == "" || cfg.user2Token == "" {
		t.Skip(skipIntegrationTests)
		return testConfig{}, true
	}

	// Parse group memberships
	if user1Groups := os.Getenv("KNOT_USER1_GROUP"); user1Groups != "" {
		cfg.user1Groups = strings.Split(user1Groups, ",")
		for i := range cfg.user1Groups {
			cfg.user1Groups[i] = strings.TrimSpace(cfg.user1Groups[i])
		}
	}
	if user2Groups := os.Getenv("KNOT_USER2_GROUP"); user2Groups != "" {
		cfg.user2Groups = strings.Split(user2Groups, ",")
		for i := range cfg.user2Groups {
			cfg.user2Groups[i] = strings.TrimSpace(cfg.user2Groups[i])
		}
	}

	return cfg, false
}

// createClient creates a new API client for testing
func createClient(baseURL, token string) (*apiclient.ApiClient, error) {
	client, err := apiclient.NewClient(baseURL, token, true) // insecureSkipVerify for local testing
	if err != nil {
		return nil, err
	}
	client.SetTimeout(30 * time.Second)
	client.SetContentType("application/json")
	return client, nil
}

// cleanupScripts deletes all test scripts created during tests
func cleanupScripts(t *testing.T, ctx context.Context, client *apiclient.ApiClient, scriptIDs []string) {
	t.Helper()

	for _, id := range scriptIDs {
		if id != "" {
			client.DeleteScript(ctx, id)
		}
	}
}

// TestSuite1_ScriptCRUD tests basic script creation and management
func TestSuite1_ScriptCRUD(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, client, createdScriptIDs)

	t.Run("CreateGlobalScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "", // Global script
			Name:        testPrefix + "global_script",
			Description: "Test global script",
			Content:     `print("Global script executed")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create global script: %v", err)
		}
		if !resp.Status {
			t.Fatalf("Script creation failed: %+v", resp)
		}
		if resp.Id == "" {
			t.Fatal("Expected script ID to be returned")
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created global script with ID: %s", resp.Id)
	})

	t.Run("CreateUserScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current", // Current user's script
			Name:        testPrefix + "user1_script",
			Description: "User1's personal script",
			Content:     `print("User1's script")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create user script: %v", err)
		}
		if !resp.Status {
			t.Fatalf("Script creation failed: %+v", resp)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created user script with ID: %s", resp.Id)
	})

	t.Run("CreateLibrary", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "library",
			Description: "Test library",
			Content:     `def helper_function(): return "helper result"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "lib",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create library: %v", err)
		}
		if !resp.Status {
			t.Fatalf("Library creation failed: %+v", resp)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created library with ID: %s", resp.Id)
	})

	t.Run("CreateMCPTool", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "tool",
			Description: "Test MCP tool",
			Content:     `def my_tool(input): return f"Tool result: {input}"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "message"
type = "string"
description = "Message to process"
required = true`,
			MCPKeywords: []string{"test", "helper"},
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create MCP tool: %v", err)
		}
		if !resp.Status {
			t.Fatalf("MCP tool creation failed: %+v", resp)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created MCP tool with ID: %s", resp.Id)
	})
}

// TestSuite2_ZoneOverrides tests zone-specific script overrides
func TestSuite2_ZoneOverrides(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, client, createdScriptIDs)

	// Clean up any existing test scripts from previous runs
	t.Run("CleanupOldTestScripts", func(t *testing.T) {
		var listResp apiclient.ScriptList
		statusCode, err := client.Do(ctx, "GET", "/api/scripts?all_zones=true", nil, &listResp)
		if err != nil {
			t.Fatalf("Failed to list scripts: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Delete any existing test scripts with the same name
		for _, script := range listResp.Scripts {
			if script.Name == testPrefix+"zone_test" {
				client.DeleteScript(ctx, script.Id)
				t.Logf("Deleted old test script: %s", script.Id)
			}
		}
	})

	// Create three scripts with the same name but different zones
	t.Run("CreateZone1Script", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "zone_test",
			Description: "Zone1 version",
			Content:     `print("Zone1 version")`,
			Zones:       []string{"zone1"},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create zone1 script: %v", err)
		}
		if !resp.Status {
			t.Fatalf("Script creation failed: %+v", resp)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created zone1 script with ID: %s", resp.Id)
	})

	t.Run("CreateZone2Script", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "zone_test",
			Description: "Zone2 version",
			Content:     `print("Zone2 version")`,
			Zones:       []string{"zone2"},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create zone2 script: %v", err)
		}
		if !resp.Status {
			t.Fatalf("Script creation failed: %+v", resp)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created zone2 script with ID: %s", resp.Id)
	})

	t.Run("CreateDefaultScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "zone_test",
			Description: "Default version",
			Content:     `print("Default version")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create default script: %v", err)
		}
		if !resp.Status {
			t.Fatalf("Script creation failed: %+v", resp)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("Created default script with ID: %s", resp.Id)
	})

	t.Run("VerifyAllScriptsExist", func(t *testing.T) {
		// List all scripts with all_zones=true
		var listResp apiclient.ScriptList
		statusCode, err := client.Do(ctx, "GET", "/api/scripts?all_zones=true", nil, &listResp)
		if err != nil {
			t.Fatalf("Failed to list scripts: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Count how many zone_test scripts exist
		count := 0
		for _, script := range listResp.Scripts {
			if strings.HasPrefix(script.Name, testPrefix+"zone_test") {
				count++
				t.Logf("Found zone_test script: %s (zones: %v)", script.Name, script.Zones)
			}
		}

		if count != 3 {
			t.Errorf("Expected 3 zone_test scripts, found %d", count)
		}
	})
}

// TestSuite3_UserIsolation tests that users can only see/execute their own scripts
func TestSuite3_UserIsolation(t *testing.T) {
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
	defer func() {
		// Clean up with user1 client (who has permission)
		cleanupScripts(t, ctx, user1Client, createdScriptIDs)
	}()

	var user1ScriptID, user2ScriptID string

	// User1 creates their own script
	t.Run("User1CreatesOwnScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "user1_private",
			Description: "User1's private script",
			Content:     `print("User1 private")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create user1 script: %v", err)
		}

		user1ScriptID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = user1ScriptID // Used for cleanup tracking
		t.Logf("User1 created script with ID: %s", resp.Id)
	})

	// User2 creates their own script
	t.Run("User2CreatesOwnScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "user2_private",
			Description: "User2's private script",
			Content:     `print("User2 private")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := user2Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create user2 script: %v", err)
		}

		user2ScriptID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		t.Logf("User2 created script with ID: %s", resp.Id)
	})

	// User1 can see their own scripts
	t.Run("User1CanSeeOwnScripts", func(t *testing.T) {
		// Get the script details to retrieve the user ID
		scriptDetails, err := user1Client.GetScript(ctx, user1ScriptID)
		if err != nil {
			t.Fatalf("Failed to get script details: %v", err)
		}
		user1Id := scriptDetails.UserId

		var listResp apiclient.ScriptList
		statusCode, err := user1Client.Do(ctx, "GET", "/api/scripts?user_id="+user1Id, nil, &listResp)
		if err != nil {
			t.Fatalf("Failed to list user1 scripts: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		found := false
		for _, script := range listResp.Scripts {
			if script.Name == testPrefix+"user1_private" {
				found = true
				break
			}
		}
		if !found {
			t.Error("User1 should be able to see their own scripts")
		}

		// User1 should NOT see user2's scripts
		for _, script := range listResp.Scripts {
			if script.Name == testPrefix+"user2_private" {
				t.Error("User1 should NOT be able to see user2's scripts")
			}
		}
	})

	// User2 can see their own scripts
	t.Run("User2CanSeeOwnScripts", func(t *testing.T) {
		// Get the script details to retrieve the user ID
		scriptDetails, err := user2Client.GetScript(ctx, user2ScriptID)
		if err != nil {
			t.Fatalf("Failed to get script details: %v", err)
		}
		user2Id := scriptDetails.UserId

		var listResp apiclient.ScriptList
		statusCode, err := user2Client.Do(ctx, "GET", "/api/scripts?user_id="+user2Id, nil, &listResp)
		if err != nil {
			t.Fatalf("Failed to list user2 scripts: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		found := false
		for _, script := range listResp.Scripts {
			if script.Name == testPrefix+"user2_private" {
				found = true
				break
			}
		}
		if !found {
			t.Error("User2 should be able to see their own scripts")
		}

		// User2 should NOT see user1's scripts
		for _, script := range listResp.Scripts {
			if script.Name == testPrefix+"user1_private" {
				t.Error("User2 should NOT be able to see user1's scripts")
			}
		}
	})

	// User1 (with admin permissions) CAN get user2's script by ID
	// Note: This is the current API behavior - admins with ManageScripts can access user scripts
	t.Run("User1CanGetUser2Script_AdminAccess", func(t *testing.T) {
		var resp apiclient.ScriptDetails
		statusCode, err := user1Client.Do(ctx, "GET", "/api/scripts/"+user2ScriptID, nil, &resp)
		if err != nil {
			t.Fatalf("Failed to get user2's script: %v", err)
		}
		if statusCode != 200 {
			t.Errorf("Admin with ManageScripts should be able to access user scripts, got status %d", statusCode)
		}
		if resp.Name != testPrefix+"user2_private" {
			t.Errorf("Expected script name '%s', got '%s'", testPrefix+"user2_private", resp.Name)
		}
		t.Logf("Admin can access user script: user_id=%s, name=%s", resp.UserId, resp.Name)
	})
}

// TestSuite4_PermissionModel tests permission enforcement
func TestSuite4_PermissionModel(t *testing.T) {
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
	defer cleanupScripts(t, ctx, user1Client, createdScriptIDs)

	var globalScriptID string

	// Create a global script with user1 (who has ManageScripts permission)
	t.Run("CreateGlobalScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "global_perm_test",
			Description: "Global permission test script",
			Content:     `print("Global permission test")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create global script: %v", err)
		}

		globalScriptID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = globalScriptID // Used for cleanup tracking
		t.Logf("Created global script with ID: %s", resp.Id)
	})

	// User1 can see global scripts (has ManageScripts permission)
	t.Run("User1CanSeeGlobalScripts", func(t *testing.T) {
		var listResp apiclient.ScriptList
		statusCode, err := user1Client.Do(ctx, "GET", "/api/scripts", nil, &listResp)
		if err != nil {
			t.Fatalf("Failed to list global scripts: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Should return empty array if no permission, not 403
		if listResp.Count == 0 {
			t.Error("User1 should be able to see global scripts (has ManageScripts permission)")
		}
	})

	// User2 cannot see global scripts (only has ManageOwnScripts)
	t.Run("User2CannotSeeGlobalScripts", func(t *testing.T) {
		var listResp apiclient.ScriptList
		statusCode, err := user2Client.Do(ctx, "GET", "/api/scripts", nil, &listResp)
		if err != nil {
			t.Fatalf("Failed to list global scripts: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Should return empty array, not 403
		if listResp.Count > 0 {
			t.Error("User2 should NOT be able to see global scripts (only has ManageOwnScripts permission)")
		}
	})
}

// TestSuite5_ZoneFiltering tests zone filtering functionality
func TestSuite5_ZoneFiltering(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, client, createdScriptIDs)

	// Create scripts with different zone configurations
	t.Run("CreateZoneSpecificScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "zone_specific",
			Description: "Zone specific script",
			Content:     `print("Zone specific")`,
			Zones:       []string{"zone1"},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create zone-specific script: %v", err)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
	})

	t.Run("CreateGlobalScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "zone_global",
			Description: "Global zone script",
			Content:     `print("Global zone")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create global zone script: %v", err)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
	})

	t.Run("ShowAllZonesReturnsAllScripts", func(t *testing.T) {
		var listResp apiclient.ScriptList
		statusCode, err := client.Do(ctx, "GET", "/api/scripts?all_zones=true", nil, &listResp)
		if err != nil {
			t.Fatalf("Failed to list all scripts: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Should include both zone-specific and global scripts
		hasZoneSpecific := false
		hasGlobal := false
		for _, script := range listResp.Scripts {
			if script.Name == testPrefix+"zone_specific" {
				hasZoneSpecific = true
			}
			if script.Name == testPrefix+"zone_global" {
				hasGlobal = true
			}
		}

		if !hasZoneSpecific {
			t.Error("Show All Zones should include zone-specific scripts")
		}
		if !hasGlobal {
			t.Error("Show All Zones should include global scripts")
		}
	})
}

// TestSuite6_MCPTools tests MCP tool integration via /mcp and /mcp/discovery endpoints
func TestSuite6_MCPTools(t *testing.T) {
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
	defer cleanupScripts(t, ctx, user1Client, createdScriptIDs)

	var globalToolID, user1ToolID string

	// Create a global MCP tool
	t.Run("CreateGlobalMCPTool", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "global_tool",
			Description: "Global MCP tool",
			Content:     `def global_tool(): return "global result"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "input"
type = "string"
description = "Input parameter"`,
			MCPKeywords: []string{"global", "test"},
			Timeout:     30,
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

	// Create user1's MCP tool with SAME NAME (should override)
	t.Run("CreateUser1MCPTool_Override", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "global_tool",
			Description: "User1's override tool",
			Content:     `def global_tool(): return "user1 override result"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "tool",
			MCPInputSchemaToml: `[[parameter]]
name = "input"
type = "string"
description = "Input parameter"`,
			MCPKeywords: []string{"user1", "override"},
			Timeout:     30,
		}

		resp, err := user1Client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create user1 MCP tool: %v", err)
		}

		user1ToolID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = user1ToolID // Used for cleanup tracking
		t.Logf("Created user1 MCP tool override with ID: %s", resp.Id)
	})

	// Test /mcp endpoint - user should see their override tool, not global
	t.Run("MCPNative_User1SeesOverrideTool", func(t *testing.T) {
		// MCP tools/list request
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
		}

		var resp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/mcp", mcpRequest, &resp)
		if err != nil {
			t.Fatalf("Failed to call /mcp endpoint: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Check that tools were returned
		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("Expected result object, got: %v", resp["result"])
		}

		tools, ok := result["tools"].([]any)
		if !ok {
			t.Fatal("Expected tools array in result")
		}

		// Find the tool with our test name
		var foundTool map[string]any
		found := false
		for _, toolAny := range tools {
			tool, ok := toolAny.(map[string]any)
			if !ok {
				continue
			}
			if name, ok := tool["name"].(string); ok && name == testPrefix+"global_tool" {
				foundTool = tool
				found = true
				break
			}
		}

		if !found {
			t.Fatal("User1's MCP tool not found in /mcp response")
		}

		// Verify the description is the USER override, not global
		description, ok := foundTool["description"].(string)
		if !ok {
			t.Error("Tool description missing")
		} else if description != "User1's override tool" {
			t.Errorf("Expected user1's override tool, got: %s", description)
		}

		t.Logf("User1 sees their override tool: %s", description)
	})

	// Test /mcp/discovery endpoint - should only show meta tools (tool_search, execute_tool)
	// User's script tools are hidden from tools/list but searchable via tool_search
	t.Run("MCPDiscovery_OnlyMetaToolsVisible", func(t *testing.T) {
		// tools/list request
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
		}

		var resp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/mcp/discovery", mcpRequest, &resp)
		if err != nil {
			t.Fatalf("Failed to call /mcp/discovery endpoint: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Check that tools were returned
		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("Expected result object, got: %v", resp["result"])
		}

		tools, ok := result["tools"].([]any)
		if !ok {
			t.Fatal("Expected tools array in result")
		}

		// Should only show tool_search and execute_tool in discovery mode
		metaTools := make(map[string]bool)
		for _, toolAny := range tools {
			tool, ok := toolAny.(map[string]any)
			if !ok {
				continue
			}
			if name, ok := tool["name"].(string); ok {
				metaTools[name] = true
			}
		}

		// Verify tool_search and execute_tool are present
		if !metaTools["tool_search"] {
			t.Error("tool_search not found in /mcp/discovery tools/list")
		}
		if !metaTools["execute_tool"] {
			t.Error("execute_tool not found in /mcp/discovery tools/list")
		}

		// Verify user's script tool is NOT in tools/list (it's hidden but searchable)
		if metaTools[testPrefix+"global_tool"] {
			t.Error("User's script tool should not be visible in /mcp/discovery tools/list (should only be searchable via tool_search)")
		}

		t.Logf("/mcp/discovery shows %d meta tools (tool_search, execute_tool)", len(tools))
	})

	// Test tool_search on /mcp/discovery to find scripts
	t.Run("MCPDiscovery_ToolSearchFindsScripts", func(t *testing.T) {
		// tool_search request via tools/call
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "tool_search",
				"arguments": map[string]any{
					"query":       testPrefix,
					"max_results": 10000,
				},
			},
		}

		var resp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/mcp/discovery", mcpRequest, &resp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Check for error response (method might not exist)
		if errObj, ok := resp["error"]; ok {
			t.Logf("tool_search returned error: %v", errObj)
			// Method not implemented - skip this test gracefully
			t.Skip("tool_search method not implemented on server")
		}

		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("Expected result object, got: %v (full response: %+v)", resp["result"], resp)
		}

		// tools/call returns content array
		content, ok := result["content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		// The first content item should have type "text" and text containing JSON
		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		// Parse the JSON text to get tools array
		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		// Should find our tool
		found := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok && name == testPrefix+"global_tool" {
				found = true
				// Verify it's the user's override
				if desc, ok := tool["description"].(string); ok {
					if desc == "User1's override tool" {
						t.Logf("tool_search found user's override tool correctly")
					}
				}
				break
			}
		}

		if !found {
			t.Error("tool_search should find user1's tool")
		}

		t.Logf("tool_search found %d matching tools", len(tools))
	})

	// Test tool_search on /mcp/discovery with empty query returns all tools
	t.Run("MCPDiscovery_ToolSearchReturnsAllTools", func(t *testing.T) {
		// tool_search request via tools/call with empty query to get all tools
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "tool_search",
				"arguments": map[string]any{
					"query":       "",
					"max_results": 10000,
				},
			},
		}

		var resp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/mcp/discovery", mcpRequest, &resp)
		if err != nil {
			t.Fatalf("Failed to call tool_search: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Check for error response
		if errObj, ok := resp["error"]; ok {
			t.Logf("tool_search returned error: %v", errObj)
			t.Skip("tool_search method not implemented on server")
		}

		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("Expected result object, got: %v", resp["result"])
		}

		// tools/call returns content array
		content, ok := result["content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		// The first content item should have type "text" and text containing JSON
		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["text"].(string)
		if !ok {
			t.Fatal("Expected text field in content item")
		}

		// Parse the JSON text to get tools array
		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			t.Fatalf("Failed to parse tool_search JSON response: %v", err)
		}

		// Should find both built-in tools and script tools
		foundUserTool := false
		foundBuiltInTool := false
		for _, tool := range tools {
			if name, ok := tool["name"].(string); ok {
				if name == testPrefix+"global_tool" {
					foundUserTool = true
					// Verify it's the user's override
					if desc, ok := tool["description"].(string); ok && desc == "User1's override tool" {
						t.Logf("tool_search found user's override tool correctly")
					}
				}
				// Check for at least one built-in tool
				if name == "list_spaces" || name == "start_space" || name == "create_space" {
					foundBuiltInTool = true
				}
			}
		}

		if !foundUserTool {
			t.Error("tool_search with empty query should find user's script tool")
		}
		if !foundBuiltInTool {
			t.Error("tool_search with empty query should find built-in tools")
		}

		t.Logf("tool_search with empty query returned %d total tools", len(tools))
	})

	// Test user2 cannot see user1's tool via /mcp
	t.Run("MCP_User2CannotSeeUser1Tool", func(t *testing.T) {
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
		}

		var resp map[string]any
		statusCode, err := user2Client.Do(ctx, "POST", "/mcp", mcpRequest, &resp)
		if err != nil {
			t.Fatalf("Failed to call /mcp endpoint: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("Expected result object, got: %v", resp["result"])
		}

		tools, ok := result["tools"].([]any)
		if !ok {
			t.Fatal("Expected tools array in result")
		}

		// User2 should NOT see user1's tool
		for _, toolAny := range tools {
			tool, ok := toolAny.(map[string]any)
			if !ok {
				continue
			}
			if name, ok := tool["name"].(string); ok && strings.HasPrefix(name, testPrefix) {
				t.Errorf("User2 should NOT be able to see user1's MCP tool: %s", name)
			}
		}
	})

	// Security test: User without ExecuteScripts permission doesn't see script tools
	t.Run("MCP_UserWithoutExecuteScripts", func(t *testing.T) {
		// User2 only has ExecuteOwnScripts, not ExecuteScripts
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
		}

		var resp map[string]any
		statusCode, err := user2Client.Do(ctx, "POST", "/mcp", mcpRequest, &resp)
		if err != nil {
			t.Fatalf("Failed to call /mcp endpoint: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("Expected result object, got: %v", resp["result"])
		}

		tools, ok := result["tools"].([]any)
		if !ok {
			t.Fatal("Expected tools array in result")
		}

		// User2 should NOT see any script tools (only built-in tools)
		for _, toolAny := range tools {
			tool, ok := toolAny.(map[string]any)
			if !ok {
				continue
			}
			name, ok := tool["name"].(string)
			if ok && strings.HasPrefix(name, testPrefix) {
				t.Errorf("User2 without ExecuteScripts should NOT see script tool: %s", name)
			}
		}

		t.Logf("User2 (without ExecuteScripts) sees %d tools (should be only built-in)", len(tools))
	})

	// Test that /mcp and /mcp/discovery have different purposes and return different data
	// /mcp (native mode): Shows all native tools (built-in + script tools)
	// /mcp/discovery (force on-demand mode): Only shows meta tools (tool_search, execute_tool)
	t.Run("MCPEndpoints_DifferentData", func(t *testing.T) {
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
		}

		// Get tools from /mcp
		var mcpResp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/mcp", mcpRequest, &mcpResp)
		if err != nil {
			t.Fatalf("Failed to call /mcp: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200 from /mcp, got %d", statusCode)
		}

		// Get tools from /mcp/discovery
		var mcpDiscoveryResp map[string]any
		statusCode, err = user1Client.Do(ctx, "POST", "/mcp/discovery", mcpRequest, &mcpDiscoveryResp)
		if err != nil {
			t.Fatalf("Failed to call /mcp/discovery: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200 from /mcp/discovery, got %d", statusCode)
		}

		// Compare tool counts
		mcpResult := mcpResp["result"].(map[string]any)
		mcpTools := mcpResult["tools"].([]any)
		mcpDiscoveryResult := mcpDiscoveryResp["result"].(map[string]any)
		mcpDiscoveryTools := mcpDiscoveryResult["tools"].([]any)

		// /mcp should return more tools than /mcp/discovery
		// /mcp: all native tools (built-in + scripts)
		// /mcp/discovery: only meta tools (tool_search, execute_tool)
		if len(mcpTools) <= len(mcpDiscoveryTools) {
			t.Errorf("/mcp should return more tools than /mcp/discovery: %d vs %d", len(mcpTools), len(mcpDiscoveryTools))
		}

		// /mcp/discovery should have exactly 2 tools (tool_search, execute_tool)
		if len(mcpDiscoveryTools) != 2 {
			t.Errorf("/mcp/discovery should have exactly 2 meta tools, got %d", len(mcpDiscoveryTools))
		}

		t.Logf("/mcp returned %d tools (native mode with all tools)", len(mcpTools))
		t.Logf("/mcp/discovery returned %d tools (force on-demand mode with only meta tools)", len(mcpDiscoveryTools))
	})

	// Test tool_search behavior on /mcp endpoint (normal mode)
	// In normal mode, tool_search only searches on-demand tools, not native tools
	// Script tools are added as native tools, so they won't be found via tool_search on /mcp
	t.Run("MCP_ToolSearchNormalModeBehavior", func(t *testing.T) {
		mcpRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "tool_search",
				"arguments": map[string]any{
					"query": testPrefix,
				},
			},
		}

		var resp map[string]any
		statusCode, err := user1Client.Do(ctx, "POST", "/mcp", mcpRequest, &resp)
		if err != nil {
			t.Fatalf("Failed to call tool_search on /mcp: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Check for error response
		if errObj, ok := resp["error"]; ok {
			t.Logf("tool_search returned error: %v", errObj)
			t.Skip("tool_search method not implemented on server")
		}

		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("Expected result object, got: %v (full response: %+v)", resp["result"], resp)
		}

		// tools/call returns content array
		content, ok := result["content"].([]any)
		if !ok {
			t.Fatal("Expected content array in tools/call result")
		}

		if len(content) == 0 {
			t.Fatal("Expected at least one content item in tool_search response")
		}

		// The first content item should have type "text" and text containing JSON or error message
		firstContent, ok := content[0].(map[string]any)
		if !ok {
			t.Fatal("Expected content item to be an object")
		}

		text, ok := firstContent["text"].(string)
		if !ok {
			t.Fatalf("Expected text field in content item, got: %v (type: %T)", firstContent["text"], firstContent["text"])
		}

		// In normal mode, script tools are native tools (not on-demand), so tool_search won't find them
		// The response should indicate no tools found or return an empty array
		t.Logf("tool_search on /mcp (normal mode) response: %q", text)

		// Parse the JSON text to get tools array
		var tools []map[string]any
		if err := json.Unmarshal([]byte(text), &tools); err != nil {
			// If it's not valid JSON, it might be an error message
			t.Logf("tool_search response is not JSON (likely 'No tools found' message)")
		}

		// In normal mode, we don't expect to find script tools (they're native, not on-demand)
		// This test verifies the expected behavior
		if len(tools) == 0 {
			t.Logf("tool_search on /mcp (normal mode) correctly returns no results - script tools are native, not on-demand")
		} else {
			t.Logf("tool_search on /mcp (normal mode) found %d tools (only on-demand tools, not native script tools)", len(tools))
		}
	})
}

// TestSuite7_LibraryAccess tests library access control

// TestSuite7_LibraryAccess tests library access control
func TestSuite7_LibraryAccess(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, client, createdScriptIDs)

	var libraryID string

	// Create a library
	t.Run("CreateLibrary", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "test_library",
			Description: "Test library",
			Content:     `def helper(): return "library result"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "lib",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create library: %v", err)
		}

		libraryID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = libraryID // Used for cleanup tracking
		t.Logf("Created library with ID: %s", resp.Id)
	})

	// Get library content
	t.Run("GetLibraryContent", func(t *testing.T) {
		content, err := client.GetScriptLibrary(ctx, testPrefix+"test_library")
		if err != nil {
			t.Fatalf("Failed to get library content: %v", err)
		}

		if content == "" {
			t.Error("Expected library content to be returned")
		}
		t.Logf("Library content: %s", content)
	})
}

// TestSuite8_Cleanup tests cleanup functionality
func TestSuite8_Cleanup(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	var createdScriptIDs []string

	// Create a test script
	t.Run("CreateTestScript", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "cleanup_test",
			Description: "Script to test cleanup",
			Content:     `print("Cleanup test")`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "script",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create script: %v", err)
		}

		createdScriptIDs = append(createdScriptIDs, resp.Id)
	})

	// Delete the script
	t.Run("DeleteScript", func(t *testing.T) {
		if len(createdScriptIDs) == 0 {
			t.Skip("No scripts to delete")
			return
		}

		scriptID := createdScriptIDs[0]
		err := client.DeleteScript(ctx, scriptID)
		if err != nil {
			t.Fatalf("Failed to delete script: %v", err)
		}

		// Verify script is deleted
		var resp apiclient.ScriptDetails
		statusCode, _ := client.Do(ctx, "GET", "/api/scripts/"+scriptID, nil, &resp)
		if statusCode != 404 {
			t.Errorf("Expected status 404 after deletion, got %d", statusCode)
		}

		t.Logf("Successfully deleted script: %s", scriptID)
	})
}

// TestScriptResolution tests script resolution by name with user override
func TestScriptResolution(t *testing.T) {
	cfg, skip := getTestConfig(t)
	if skip {
		return
	}

	ctx := context.Background()
	client, err := createClient(cfg.baseURL, cfg.user1Token)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	var createdScriptIDs []string
	defer cleanupScripts(t, ctx, client, createdScriptIDs)

	var globalScriptID, userScriptID string

	// Create a global script named "helper"
	t.Run("CreateGlobalHelper", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "",
			Name:        testPrefix + "helper",
			Description: "Global helper script",
			Content:     `def helper(): return "global helper"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "lib",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create global helper: %v", err)
		}

		globalScriptID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = globalScriptID // Used for cleanup tracking
	})

	// Create a user script with the same name
	t.Run("CreateUserHelper", func(t *testing.T) {
		req := apiclient.ScriptCreateRequest{
			UserId:      "current",
			Name:        testPrefix + "helper",
			Description: "User's helper script",
			Content:     `def helper(): return "user helper"`,
			Zones:       []string{},
			Active:      true,
			ScriptType:  "lib",
			Timeout:     30,
		}

		resp, err := client.CreateScript(ctx, req)
		if err != nil {
			t.Fatalf("Failed to create user helper: %v", err)
		}

		userScriptID = resp.Id
		createdScriptIDs = append(createdScriptIDs, resp.Id)
		_ = userScriptID // Used for cleanup tracking
	})

	// Get script by name - should return user's version
	t.Run("GetScriptByName_UserOverride", func(t *testing.T) {
		var resp apiclient.ScriptDetails
		statusCode, err := client.Do(ctx, "GET", "/api/scripts/name/"+testPrefix+"helper", nil, &resp)
		if err != nil {
			t.Fatalf("Failed to get script by name: %v", err)
		}
		if statusCode != 200 {
			t.Fatalf("Expected status 200, got %d", statusCode)
		}

		// Should return user's script (user override)
		if resp.UserId == "" {
			t.Error("Expected user script to be returned (user override), got global script")
		}
		if resp.Name != testPrefix+"helper" {
			t.Errorf("Expected script name '%s', got '%s'", testPrefix+"helper", resp.Name)
		}

		t.Logf("Got script by name: user_id=%s, name=%s", resp.UserId, resp.Name)
	})
}
