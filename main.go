package main

import (
	"os"
	"time"

	"github.com/paularlott/knot/command"
	_ "github.com/paularlott/knot/command/admin"
	_ "github.com/paularlott/knot/command/direct"
	_ "github.com/paularlott/knot/command/forward"
	_ "github.com/paularlott/knot/command/proxy"
	_ "github.com/paularlott/knot/command/spaces"
	_ "github.com/paularlott/knot/command/ssh-config"
	_ "github.com/paularlott/knot/command/templates"
	_ "github.com/paularlott/knot/command/tunnel"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC822})
	command.Execute()
}
