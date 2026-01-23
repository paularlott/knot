package apiclient

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
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

func (c *ApiClient) CreateScript(ctx context.Context, req ScriptCreateRequest) (*ScriptCreateResponse, error) {
	var resp ScriptCreateResponse
	_, err := c.httpClient.Post(ctx, "/api/scripts", req, &resp, 201)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ApiClient) UpdateScript(ctx context.Context, scriptId string, req ScriptUpdateRequest) error {
	_, err := c.httpClient.Put(ctx, "/api/scripts/"+scriptId, req, nil, 200)
	return err
}

func (c *ApiClient) ExecuteScript(ctx context.Context, spaceId, scriptId string, args []string) (string, int, error) {
	req := ScriptExecuteRequest{Arguments: args}
	var resp ScriptExecuteResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/scripts/"+scriptId+"/execute", req, &resp, 0)
	if err != nil {
		return "", 0, err
	}
	if resp.Error != "" {
		return resp.Output, resp.ExitCode, fmt.Errorf("%s", resp.Error)
	}
	return resp.Output, resp.ExitCode, nil
}

func (c *ApiClient) GetScriptLibrary(ctx context.Context, name string) (string, error) {
	var content string
	_, err := c.httpClient.Get(ctx, "/api/scripts/name/"+name+"/lib", &content)
	if err != nil {
		return "", err
	}
	return content, nil
}

func (c *ApiClient) ExecuteScriptContent(ctx context.Context, spaceId, content string, args []string) (string, int, error) {
	req := ScriptContentExecuteRequest{Content: content, Arguments: args}
	var resp ScriptExecuteResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/execute-content", req, &resp, 0)
	if err != nil {
		return "", 0, err
	}
	if resp.Error != "" {
		return resp.Output, resp.ExitCode, fmt.Errorf("%s", resp.Error)
	}
	return resp.Output, resp.ExitCode, nil
}

func (c *ApiClient) ExecuteScriptByName(ctx context.Context, spaceId, scriptName string, args []string) (string, int, error) {
	req := ScriptNameExecuteRequest{ScriptName: scriptName, Arguments: args}
	var resp ScriptExecuteResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/execute-script-name", req, &resp, 0)
	if err != nil {
		return "", 0, err
	}
	if resp.Error != "" {
		return resp.Output, resp.ExitCode, fmt.Errorf("%s", resp.Error)
	}
	return resp.Output, resp.ExitCode, nil
}

func (c *ApiClient) ExecuteScriptStream(ctx context.Context, spaceId, scriptName string, args []string) (int, error) {
	return c.executeScriptStream(ctx, spaceId, scriptName, "", args)
}

func (c *ApiClient) ExecuteScriptContentStream(ctx context.Context, spaceId, content string, args []string) (int, error) {
	return c.executeScriptStream(ctx, spaceId, "", content, args)
}

func (c *ApiClient) executeScriptStream(ctx context.Context, spaceId, scriptName, content string, args []string) (int, error) {
	var url string
	if content != "" {
		url = fmt.Sprintf("%s/api/spaces/%s/execute-script-stream?content=true", c.GetWebSocketURL(), spaceId)
	} else {
		url = fmt.Sprintf("%s/api/spaces/%s/execute-script-stream?script=%s", c.GetWebSocketURL(), spaceId, scriptName)
	}
	for _, arg := range args {
		url += "&arg=" + arg
	}

	ws, err := c.ConnectWebSocket(ctx, url)
	if err != nil {
		return 1, err
	}
	defer ws.Close()

	if content != "" {
		if err := ws.WriteMessage(websocket.TextMessage, []byte(content)); err != nil {
			return 1, err
		}
	}

	exitCode := 0
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		<-sigChan
		ws.WriteMessage(websocket.TextMessage, []byte("stop"))
		closeDone()
		ws.Close() // Force close to unblock ReadMessage
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := os.Stdin.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
						return
					}
				}
			}
		}
	}()

	for {
		msgType, data, err := ws.ReadMessage()
		if err != nil {
			return exitCode, nil
		}
		if msgType == websocket.TextMessage {
			if strings.HasPrefix(string(data), "exit:") {
				fmt.Sscanf(string(data), "exit:%d", &exitCode)
				closeDone()
				return exitCode, nil
			}
		} else if msgType == websocket.BinaryMessage {
			os.Stdout.Write(data)
		}
	}
}
