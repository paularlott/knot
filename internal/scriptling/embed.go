package scriptling

import "embed"

//go:embed lib/knot/*.py
var EmbeddedLibs embed.FS
