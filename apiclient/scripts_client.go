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

func (c *ApiClient) GetScriptByName(ctx context.Context, name string) (*ScriptDetails, error) {
	scripts, err := c.GetScripts(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range scripts.Scripts {
		if s.Name == name {
			return c.GetScript(ctx, s.Id)
		}
	}
	return nil, fmt.Errorf("script not found")
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

func (c *ApiClient) GetScriptLibraries(ctx context.Context) (map[string]string, error) {
	scripts, err := c.GetScripts(ctx)
	if err != nil {
		return nil, err
	}
	libraries := make(map[string]string)
	for _, s := range scripts.Scripts {
		if s.ScriptType == "lib" {
			script, err := c.GetScript(ctx, s.Id)
			if err == nil {
				libraries[s.Name] = script.Content
			}
		}
	}
	return libraries, nil
}
