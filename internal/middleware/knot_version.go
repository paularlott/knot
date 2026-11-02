package middleware

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/util/rest"
)

// knotUserAgent matches the User-Agent the knot command sends.
var knotUserAgent = regexp.MustCompile(`^knot v(\d+\.\d+)\.`)

// KnotVersion stamps every reply with the server's version and refuses
// requests from a knot command on another major.minor version: the two
// name fields on the wire alike only within a minor version, so a mismatch
// would otherwise lose fields without a word.
func KnotVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(build.VersionHeader, build.Version)
		if m := knotUserAgent.FindStringSubmatch(r.UserAgent()); m != nil && m[1] != build.MajorMinor(build.Version) {
			rest.WriteResponse(http.StatusBadRequest, w, r, struct {
				Error string `json:"error" msgpack:"error"`
			}{Error: fmt.Sprintf("this knot command is version %s, but the server is %s; use a knot command from version %s", m[1], build.Version, build.MajorMinor(build.Version))})
			return
		}
		next.ServeHTTP(w, r)
	})
}
