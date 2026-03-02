package agent_client

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/evaliface"
	"github.com/paularlott/scriptling/object"
)

const consoleLibraryName = "scriptling.console"
const nativeStubKey = "__stub__"

// consoleStub holds the framed stream writer and inbound control channel for one Console instance.
type consoleStub struct {
	w        io.Writer // raw mux stream (for FrameControl)
	inbound  <-chan string
	mu       sync.Mutex
	submitCb func(context.Context, string)
	escapeCb func()
	commands map[string]func(context.Context, string) // name → handler
	cancel   context.CancelFunc
	prevDone chan struct{}
	prebuf   []string // control messages sent before run()
	running  bool
}

func (s *consoleStub) Type() object.ObjectType                           { return object.BUILTIN_OBJ }
func (s *consoleStub) Inspect() string                                   { return "<Console>" }
func (s *consoleStub) AsString() (string, object.Object)                 { return "<Console>", nil }
func (s *consoleStub) AsInt() (int64, object.Object)                     { return 0, nil }
func (s *consoleStub) AsFloat() (float64, object.Object)                 { return 0, nil }
func (s *consoleStub) AsBool() (bool, object.Object)                     { return true, nil }
func (s *consoleStub) AsList() ([]object.Object, object.Object)          { return nil, nil }
func (s *consoleStub) AsDict() (map[string]object.Object, object.Object) { return nil, nil }
func (s *consoleStub) CoerceString() (string, object.Object)             { return "<Console>", nil }
func (s *consoleStub) CoerceInt() (int64, object.Object)                 { return 0, nil }
func (s *consoleStub) CoerceFloat() (float64, object.Object)             { return 0, nil }

func (s *consoleStub) send(op string, payload interface{}) {
	var msg string
	if payload != nil {
		b, _ := json.Marshal(payload)
		msg = op + ":" + string(b)
	} else {
		msg = op
	}
	s.mu.Lock()
	running := s.running
	if !running {
		s.prebuf = append(s.prebuf, msg)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	WriteFrame(s.w, FrameControl, []byte(msg))
}

func stubFrom(args []object.Object) *consoleStub {
	return args[0].(*object.Instance).Fields[nativeStubKey].(*consoleStub)
}

func envFromCtx(ctx context.Context) *object.Environment {
	if env, ok := ctx.Value("scriptling-env").(*object.Environment); ok {
		return env
	}
	return object.NewEnvironment()
}

// newConsoleClass builds the Console class backed by the given stub factory.
func newConsoleClass(w io.Writer, inbound <-chan string) *object.Class {
	return &object.Class{
		Name: "Console",
		Methods: map[string]object.Object{
			"__init__": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					inst := args[0].(*object.Instance)
					stub := &consoleStub{
						w:        w,
						inbound:  inbound,
						commands: make(map[string]func(context.Context, string)),
						prevDone: make(chan struct{}),
					}
					close(stub.prevDone)
					inst.Fields[nativeStubKey] = stub
					return &object.Null{}
				},
			},
			"add_message": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					s := stubFrom(args)
					parts := make([]string, len(args)-1)
					for i, a := range args[1:] {
						parts[i] = a.Inspect()
					}
					label, _ := kwargs.GetString("label", "")
					s.send("add_message", map[string]string{"text": strings.Join(parts, " "), "label": label})
					return &object.Null{}
				},
			},
			"stream_start": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					label, _ := kwargs.GetString("label", "")
					stubFrom(args).send("stream_start", map[string]string{"label": label})
					return &object.Null{}
				},
			},
			"stream_chunk": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					if len(args) > 1 {
						if text, err := args[1].AsString(); err == nil {
							stubFrom(args).send("stream_chunk", map[string]string{"text": text})
						}
					}
					return &object.Null{}
				},
			},
			"stream_end": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					stubFrom(args).send("stream_end", nil)
					return &object.Null{}
				},
			},
			"spinner_start": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					text := "Working"
					if len(args) > 1 {
						if s, err := args[1].AsString(); err == nil {
							text = s
						}
					}
					stubFrom(args).send("spinner_start", map[string]string{"text": text})
					return &object.Null{}
				},
			},
			"spinner_stop": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					stubFrom(args).send("spinner_stop", nil)
					return &object.Null{}
				},
			},
			"set_progress": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					label := ""
					pct := -1.0
					if len(args) > 1 {
						if s, err := args[1].AsString(); err == nil {
							label = s
						}
					}
					if len(args) > 2 {
						if f, err := args[2].AsFloat(); err == nil {
							pct = f
						}
					}
					stubFrom(args).send("set_progress", map[string]interface{}{"label": label, "pct": pct})
					return &object.Null{}
				},
			},
			"set_labels": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					user, assistant, system := "", "", ""
					if len(args) > 1 {
						if s, err := args[1].AsString(); err == nil {
							user = s
						}
					}
					if len(args) > 2 {
						if s, err := args[2].AsString(); err == nil {
							assistant = s
						}
					}
					if len(args) > 3 {
						if s, err := args[3].AsString(); err == nil {
							system = s
						}
					}
					stubFrom(args).send("set_labels", map[string]string{"user": user, "assistant": assistant, "system": system})
					return &object.Null{}
				},
			},
			"set_status": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					left, right := "", ""
					if len(args) > 1 {
						if s, err := args[1].AsString(); err == nil {
							left = s
						}
					}
					if len(args) > 2 {
						if s, err := args[2].AsString(); err == nil {
							right = s
						}
					}
					stubFrom(args).send("set_status", map[string]string{"left": left, "right": right})
					return &object.Null{}
				},
			},
			"set_status_left": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					if len(args) > 1 {
						if s, err := args[1].AsString(); err == nil {
							stubFrom(args).send("set_status_left", map[string]string{"text": s})
						}
					}
					return &object.Null{}
				},
			},
			"set_status_right": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					if len(args) > 1 {
						if s, err := args[1].AsString(); err == nil {
							stubFrom(args).send("set_status_right", map[string]string{"text": s})
						}
					}
					return &object.Null{}
				},
			},
			"register_command": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					if len(args) < 4 {
						return &object.Null{}
					}
					name, err := args[1].AsString()
					if err != nil {
						return err
					}
					desc, err := args[2].AsString()
					if err != nil {
						return err
					}
					fn := args[3]
					eval := evaliface.FromContext(ctx)
					env := envFromCtx(ctx)
					s := stubFrom(args)
					s.mu.Lock()
					s.commands[name] = func(cmdCtx context.Context, cmdArgs string) {
						if eval != nil {
							eval.CallObjectFunction(cmdCtx, fn,
								[]object.Object{&object.String{Value: cmdArgs}}, nil, env)
						}
					}
					s.mu.Unlock()
					s.send("register_command", map[string]string{"name": name, "desc": desc})
					return &object.Null{}
				},
			},
			"remove_command": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					if len(args) > 1 {
						if name, err := args[1].AsString(); err == nil {
							s := stubFrom(args)
							s.mu.Lock()
							delete(s.commands, name)
							s.mu.Unlock()
							s.send("remove_command", map[string]string{"name": name})
						}
					}
					return &object.Null{}
				},
			},
			"clear_output": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					stubFrom(args).send("clear_output", nil)
					return &object.Null{}
				},
			},
			"styled": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					// No local TUI — return text unstyled
					if len(args) < 3 {
						return &object.String{Value: ""}
					}
					text, err := args[2].AsString()
					if err != nil {
						return err
					}
					return &object.String{Value: text}
				},
			},
			"on_escape": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					if len(args) > 1 {
						fn := args[1]
						eval := evaliface.FromContext(ctx)
						env := envFromCtx(ctx)
						s := stubFrom(args)
						s.mu.Lock()
						s.escapeCb = func() {
							if eval != nil {
								eval.CallObjectFunction(context.Background(), fn, nil, nil, env)
							}
						}
						s.mu.Unlock()
					}
					return &object.Null{}
				},
			},
			"on_submit": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					if len(args) > 1 {
						fn := args[1]
						eval := evaliface.FromContext(ctx)
						env := envFromCtx(ctx)
						s := stubFrom(args)
						s.mu.Lock()
						s.submitCb = func(submitCtx context.Context, text string) {
							if eval != nil {
								eval.CallObjectFunction(submitCtx, fn,
									[]object.Object{&object.String{Value: text}}, nil, env)
							}
						}
						s.mu.Unlock()
					}
					return &object.Null{}
				},
			},
			"run": &object.Builtin{
				Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
					s := stubFrom(args)
					s.mu.Lock()
					s.running = true
					prebuf := s.prebuf
					s.prebuf = nil
					s.mu.Unlock()
					WriteFrame(s.w, FrameControl, []byte("tui:start"))
					for _, msg := range prebuf {
						WriteFrame(s.w, FrameControl, []byte(msg))
					}
					// Dispatch inbound control messages until tui:end
					for msg := range s.inbound {
						if msg == "tui:end" || msg == "stop" {
							break
						}
						if strings.HasPrefix(msg, "submit:") {
							text := strings.TrimPrefix(msg, "submit:")
							s.mu.Lock()
							scb := s.submitCb
							ecb := s.escapeCb
							if s.cancel != nil {
								s.cancel()
								if ecb != nil {
									go ecb()
								}
							}
							submitCtx, c := context.WithCancel(context.Background())
							s.cancel = c
							waitFor := s.prevDone
							nextDone := make(chan struct{})
							s.prevDone = nextDone
							s.mu.Unlock()
							if scb != nil {
								go func(cb func(context.Context, string), t string) {
									defer func() {
										s.mu.Lock()
										s.cancel = nil
										s.mu.Unlock()
										c()
										close(nextDone)
									}()
									<-waitFor
									cb(submitCtx, t)
								}(scb, text)
							} else {
								c()
								close(nextDone)
							}
						} else if msg == "escape" {
							s.mu.Lock()
							if s.cancel != nil {
								s.cancel()
							}
							cb := s.escapeCb
							s.mu.Unlock()
							if cb != nil {
								go cb()
							}
						} else if strings.HasPrefix(msg, "command:") {
							rest := strings.TrimPrefix(msg, "command:")
							colon := strings.Index(rest, ":")
							var name, cmdArgs string
							if colon >= 0 {
								name, cmdArgs = rest[:colon], rest[colon+1:]
							} else {
								name = rest
							}
							s.mu.Lock()
							cb := s.commands[name]
							s.mu.Unlock()
							if cb != nil {
								go cb(context.Background(), cmdArgs)
							}
						}
					}
					s.send("tui:end", nil)
					return &object.Null{}
				},
			},
		},
	}
}

// registerConsoleStub registers the stub scriptling.console library using the given framed writer
// and inbound control message channel.
func registerConsoleStub(
	registrar interface{ RegisterLibrary(*object.Library) },
	w io.Writer,
	inbound <-chan string,
) {
	lib := object.NewLibrary(consoleLibraryName, nil, map[string]object.Object{
		"Console":   newConsoleClass(w, inbound),
		"PRIMARY":   &object.String{Value: "primary"},
		"SECONDARY": &object.String{Value: "secondary"},
		"ERROR":     &object.String{Value: "error"},
		"DIM":       &object.String{Value: "dim"},
		"USER":      &object.String{Value: "user"},
		"TEXT":      &object.String{Value: "text"},
	}, "Console I/O with TUI backend (remote stub)")
	registrar.RegisterLibrary(lib)
}

// stubError satisfies object.Object for error returns — reuse errors package
var _ = errors.NewError
