package apiclient

import (
	"encoding/json"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/paularlott/cli/tui"
)

type tuiMsg struct {
	stdout []byte
	ctrl   string
}

// startTUI creates a TUI in read-only mode and returns it along with the input
// channel and a done channel that closes when the dispatch goroutine exits.
// The caller must call t.Run() on the main goroutine.
func startTUI(ws *websocket.Conn) (*tui.TUI, chan tuiMsg, chan struct{}) {
	inputOn := true
	var t *tui.TUI
	t = tui.New(tui.Config{
		InputEnabled: &inputOn,
		SystemLabel:  "",
		Commands: []*tui.Command{
			{Name: "exit", Description: "Exit", Handler: func(_ string) {
				ws.WriteMessage(websocket.TextMessage, []byte("stop"))
				ws.Close()
			}},
		},
		OnEscape: func() {
			ws.WriteMessage(websocket.TextMessage, []byte("escape"))
		},
		OnSubmit: func(text string) {
			t.AddMessage(tui.RoleUser, text)
			ws.WriteMessage(websocket.TextMessage, []byte("submit:"+text))
		},
	})

	in := make(chan tuiMsg, 64)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for msg := range in {
			if msg.stdout != nil {
				t.StreamChunk(strings.TrimRight(string(msg.stdout), "\r\n"))
			} else {
				dispatchToTUI(t, ws, msg.ctrl)
			}
		}
		t.StreamComplete()
		t.Exit()
	}()

	return t, in, done
}

type msgPayload struct {
	Text      string  `json:"text"`
	Label     string  `json:"label"`
	Left      string  `json:"left"`
	Right     string  `json:"right"`
	User      string  `json:"user"`
	Assistant string  `json:"assistant"`
	System    string  `json:"system"`
	Pct       float64 `json:"pct"`
	Name      string  `json:"name"`
	Desc      string  `json:"desc"`
}

func dispatchToTUI(t *tui.TUI, ws *websocket.Conn, msg string) {
	if msg == "tui:start" {
		t.SetInputEnabled(true)
		return
	}
	if msg == "tui:end" {
		t.SetInputEnabled(false)
		return
	}
	if msg == "spinner_stop" {
		t.StopSpinner()
		return
	}
	if msg == "stream_end" {
		t.StreamComplete()
		return
	}
	if msg == "clear_output" {
		t.ClearOutput()
		return
	}

	idx := strings.Index(msg, ":")
	if idx < 0 {
		return
	}
	op := msg[:idx]
	raw := msg[idx+1:]

	var p msgPayload
	json.Unmarshal([]byte(raw), &p)

	switch op {
	case "add_message":
		if p.Label != "" {
			t.AddMessageAs(tui.RoleAssistant, p.Label, p.Text)
		} else {
			t.AddMessage(tui.RoleAssistant, p.Text)
		}
	case "stream_start":
		if p.Label != "" {
			t.StartStreamingAs(p.Label)
		} else {
			t.StartStreaming()
		}
	case "stream_chunk":
		t.StreamChunk(p.Text)
	case "spinner_start":
		t.StartSpinner(p.Text)
	case "set_progress":
		if p.Pct < 0 {
			t.ClearProgress()
		} else {
			t.SetProgress(p.Label, p.Pct)
		}
	case "set_labels":
		t.SetLabels(p.User, p.Assistant, p.System)
	case "set_status":
		t.SetStatus(p.Left, p.Right)
	case "set_status_left":
		t.SetStatusLeft(p.Text)
	case "set_status_right":
		t.SetStatusRight(p.Text)
	case "register_command":
		name := p.Name
		t.AddCommand(&tui.Command{
			Name:        name,
			Description: p.Desc,
			Handler: func(args string) {
				ws.WriteMessage(websocket.TextMessage, []byte("command:"+name+":"+args))
			},
		})
	case "remove_command":
		t.RemoveCommand(p.Name)
	}
}
