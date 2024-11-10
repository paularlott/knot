package main

import (
	"os"
	"time"

	"github.com/paularlott/knot/agent/agentcmd"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC822})
	agentcmd.Execute()
}
