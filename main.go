package main

import (
	"os"
	"time"

	"github.com/paularlott/knot/command"
	_ "github.com/paularlott/knot/command/agent"
	_ "github.com/paularlott/knot/command/direct"
	_ "github.com/paularlott/knot/command/forward"
	_ "github.com/paularlott/knot/command/proxy"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC822})
  command.Execute()
}
