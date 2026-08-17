//go:build pro

package harness

// ProBuild reports whether the harness is running against a pro build
// (server built with the `pro` tag). Pro builds include the pro:* feature
// areas in the report catalog.
const ProBuild = true
