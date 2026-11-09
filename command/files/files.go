package command_files

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
		snapshotCmd,
		catCmd,
		rmCmd,
		syncCmd,
		syncProjectCmd,
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
		return nil, cmdutil.ErrNoServer
	}
	skipVerify := cmd.GetBool("tls-skip-verify") || agentlink.IsAgentRunning()
	return apiclient.NewClient(addr.HttpServer, addr.ApiToken, skipVerify)
}

// apiError makes an API error readable.
func apiError(err error) error {
	return cmdutil.CleanErr(err)
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
	if link := linkOnPath(dir, rel); link != "" {
		return "", fmt.Errorf("refusing to write %s through %s, a symbolic link", rel, link)
	}
	return local, nil
}

// linkOnPath returns the first folder on the way from dir to rel that is a
// symbolic link, "" when none is: what is written there would land where the
// link points, perhaps outside dir. rel itself may be a link, which is
// replaced rather than followed.
func linkOnPath(dir, rel string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "/")
	p := dir
	for i, part := range parts[:len(parts)-1] {
		p = filepath.Join(p, part)
		info, err := os.Lstat(p)
		if err != nil {
			return "" // not there yet, so nothing to pass through
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return strings.Join(parts[:i+1], "/")
		}
	}
	return ""
}

// linksSupported says whether symbolic links are copied as links: not on
// Windows, where making one takes a privilege most users lack.
var linksSupported = runtime.GOOS != "windows"

// linkMode is the mode a symbolic link is stored with, as the NFS server and
// rclone store one.
const linkMode = os.ModeSymlink | 0777

// maxLinkTarget bounds a link's target, PATH_MAX on Linux.
const maxLinkTarget = 4096

// makeLink makes local a symbolic link to target, replacing a file or link
// there in one step.
func makeLink(target, local string) error {
	if !linksSupported {
		return fmt.Errorf("%s: symbolic links are not supported here", local)
	}
	if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(local), fmt.Sprintf(".knot-link-%d-%s", os.Getpid(), filepath.Base(local)))
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, local); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
