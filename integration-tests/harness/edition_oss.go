//go:build !pro

package harness

// ProBuild reports whether the harness is running against a pro build
// (server built with the `pro` tag). OSS builds omit the pro:* feature
// areas from the report catalog.
const ProBuild = false
