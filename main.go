package main

import (
	"github.com/paularlott/knot/cmd"
	_ "github.com/paularlott/knot/cmd/forward"
	_ "github.com/paularlott/knot/cmd/proxy"
)

func main() {
  cmd.Execute()
}
