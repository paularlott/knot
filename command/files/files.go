package command_files

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
)

// FilesCmd is `knot file`: object storage in buckets replicated across the
// cluster. It works from the desktop (via the configured server) and inside
// a space (via the agent's connection to its server).
var FilesCmd = &cli.Command{
	Name:  "file",
	Usage: "Manage file storage",
	Description: `Store files in buckets replicated across the knot cluster.

A bucket's files are written bucket:path, e.g. configs:app/settings.toml, and any other path is local. Buckets are private to their owner unless shared; manage them with knot file bucket.

Knot Pro also serves the same buckets over S3 at <server>/s3, using your username as the access key and an API key as the secret key.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: []string{"s"},
			Usage:   "The address of the remote server.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
			Global:  true,
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "The token to use for authentication.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
			Global:  true,
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use from the desktop; inside a space the space's server is used unless an alias is given.",
			DefaultValue: "default",
			Global:       true,
		},
	},
	Commands: []*cli.Command{
		lsCmd,
		copyCmd,
		moveCmd,
		catCmd,
		rmCmd,
		syncCmd,
		usageCmd,
		bucketCmd,
	},
}

// getClient connects to the knot server the same way as knot mcp and
// knot tunnel: an explicit --server/--token pair, else inside a space the
// agent's own connection (server and credentials are discovered, nothing to
// configure), else on the desktop the --alias connection written by knot
// connect. An --alias given explicitly inside a space targets that server.
func getClient(cmd *cli.Command) (*apiclient.ApiClient, error) {
	addr := cmdutil.GetServerAddr(cmd)
	if addr == nil || addr.HttpServer == "" {
		return nil, fmt.Errorf("no server configured, use knot connect or --alias")
	}
	skipVerify := cmd.GetBool("tls-skip-verify") || agentlink.IsAgentRunning()
	return apiclient.NewClient(addr.HttpServer, addr.ApiToken, skipVerify)
}

// apiError makes an API error readable.
func apiError(err error) error {
	return errors.New(cmdutil.CleanAPIError(err))
}

// dirPrefix makes a non-empty key prefix end in "/", so it names a folder.
func dirPrefix(prefix string) string {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

// within returns where the bucket-relative path rel goes below dir,
// refusing a key that would land outside it.
func within(dir, rel string) (string, error) {
	local := filepath.Join(dir, filepath.FromSlash(rel))
	if r, err := filepath.Rel(dir, local); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write %s outside %s", rel, dir)
	}
	return local, nil
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// jsonFlag makes a command print its result as JSON for scripts.
var jsonFlag = &cli.BoolFlag{Name: "json", Usage: "Print the result as JSON."}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
