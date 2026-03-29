package apiclient

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
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
	statusCode, err := c.httpClient.Get(ctx, "/api/scripts/name/"+name+"/script", &content)
	if err != nil {
		return "", err
	}
	if statusCode == 404 {
		return "", fmt.Errorf("script not found: %s", name)
	}
	if statusCode >= 400 {
		return "", fmt.Errorf("unexpected status code: %d", statusCode)
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
	statusCode, err := c.httpClient.Get(ctx, "/api/scripts/name/"+name+"/lib", &content)
	if err != nil {
		return "", err
	}
	if statusCode == 404 {
		return "", fmt.Errorf("library not found: %s", name)
	}
	if statusCode >= 400 {
		return "", fmt.Errorf("unexpected status code: %d", statusCode)
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
	req := UnifiedScriptExecuteRequest{ScriptName: scriptName, Arguments: args}
	var resp ScriptExecuteResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/execute-script", req, &resp, 0)
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

	// Forward piped stdin as binary frames
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					ws.WriteMessage(websocket.BinaryMessage, buf[:n])
				}
				if err != nil {
					if err != io.EOF {
						ws.WriteMessage(websocket.TextMessage, []byte("stop"))
					} else {
						ws.WriteMessage(websocket.TextMessage, []byte("stdin_eof"))
					}
					return
				}
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		<-sigChan
		ws.WriteMessage(websocket.TextMessage, []byte("stop"))
		ws.Close()
	}()

	// Read loop: plain stdout until tui:start or exit.
	tuiCtx, tuiCancel := context.WithCancel(context.Background())
	defer tuiCancel()

	var pendingMsgs []tuiMsg
	isTUI := false

	processMsg := func(mt int, d []byte) {
		if mt == websocket.BinaryMessage {
			if isTUI {
				pendingMsgs = append(pendingMsgs, tuiMsg{stdout: d})
			} else {
				os.Stdout.Write(d)
			}
			return
		}
		s := string(d)
		if strings.HasPrefix(s, "exit:") {
			fmt.Sscanf(s, "exit:%d", &exitCode)
			tuiCancel()
			return
		}
		if s == "tui:start" {
			isTUI = true
		}
		pendingMsgs = append(pendingMsgs, tuiMsg{ctrl: s})
	}

	// Buffer messages until tui:start or exit, printing plain stdout along the way.
	for !isTUI {
		select {
		case <-tuiCtx.Done():
			return exitCode, nil
		default:
		}
		mt, d, err := ws.ReadMessage()
		if err != nil {
			return exitCode, nil
		}
		processMsg(mt, d)
	}

	// TUI mode.
	t, tuiIn, tuiDone := startTUI(ws)
	go func() {
		defer close(tuiIn)
		for _, m := range pendingMsgs {
			tuiIn <- m
		}
		for {
			mt, d, err := ws.ReadMessage()
			if err != nil {
				tuiCancel()
				return
			}
			if mt == websocket.BinaryMessage {
				tuiIn <- tuiMsg{stdout: d}
			} else {
				s := string(d)
				if strings.HasPrefix(s, "exit:") {
					fmt.Sscanf(s, "exit:%d", &exitCode)
					tuiCancel()
					return
				}
				tuiIn <- tuiMsg{ctrl: s}
			}
		}
	}()
	t.Run(tuiCtx)
	<-tuiDone
	return exitCode, nil
}
