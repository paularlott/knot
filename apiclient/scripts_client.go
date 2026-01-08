package apiclient

import (
	"context"
	"fmt"
)

func (c *ApiClient) GetScripts(ctx context.Context) (*ScriptList, error) {
	var scripts ScriptList
	_, err := c.httpClient.Get(ctx, "/api/scripts", &scripts)
	return &scripts, err
}

func (c *ApiClient) GetScript(ctx context.Context, id string) (*ScriptDetails, error) {
	var script ScriptDetails
	_, err := c.httpClient.Get(ctx, "/api/scripts/"+id, &script)
	return &script, err
}

func (c *ApiClient) GetScriptDetailsByName(ctx context.Context, name string) (*ScriptDetails, error) {
	var script ScriptDetails
	_, err := c.httpClient.Get(ctx, "/api/scripts/name/"+name, &script)
	return &script, err
}

func (c *ApiClient) GetScriptByName(ctx context.Context, name string) (string, error) {
	var content string
	_, err := c.httpClient.Get(ctx, "/api/scripts/name/"+name+"/script", &content)
	if err != nil {
		return "", err
	}
	return content, nil
}

func (c *ApiClient) DeleteScript(ctx context.Context, id string) error {
	_, err := c.httpClient.Delete(ctx, "/api/scripts/"+id, nil, nil, 0)
	return err
}

func (c *ApiClient) ExecuteScript(ctx context.Context, spaceId, scriptId string, args []string) (string, error) {
	req := ScriptExecuteRequest{Arguments: args}
	var resp ScriptExecuteResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/scripts/"+scriptId+"/execute", req, &resp, 0)
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return resp.Output, fmt.Errorf("%s", resp.Error)
	}
	return resp.Output, nil
}

func (c *ApiClient) GetScriptLibrary(ctx context.Context, name string) (string, error) {
	var content string
	_, err := c.httpClient.Get(ctx, "/api/scripts/name/"+name+"/lib", &content)
	if err != nil {
		return "", err
	}
	return content, nil
}

func (c *ApiClient) ExecuteScriptContent(ctx context.Context, spaceId, content string, args []string) (string, error) {
	req := ScriptContentExecuteRequest{Content: content, Arguments: args}
	var resp ScriptExecuteResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/execute-content", req, &resp, 0)
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return resp.Output, fmt.Errorf("%s", resp.Error)
	}
	return resp.Output, nil
}

func (c *ApiClient) ExecuteScriptByName(ctx context.Context, spaceId, scriptName string, args []string) (string, error) {
	req := ScriptNameExecuteRequest{ScriptName: scriptName, Arguments: args}
	var resp ScriptExecuteResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/execute-script-name", req, &resp, 0)
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return resp.Output, fmt.Errorf("%s", resp.Error)
	}
	return resp.Output, nil
}
