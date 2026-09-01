package pluginfetch

import _ "embed"

// apiclientPluginSource is the plugin-transport variant of knot.apiclient.py.
// It replaces the embedded HTTP-based copy when served by the fetcher, so
// API calls route through the plugin's registered functions (plugin.knot.*)
// and the token stays in the plugin process.
//
//go:embed lib/knot/apiclient.py
var apiclientPluginSource []byte
