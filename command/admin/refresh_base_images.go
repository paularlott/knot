package commands_admin

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/config"
)

// restErrPrefix matches the REST client's "unexpected status code: N: " wrapper
// so per-server failure lines show only the underlying server message.
var restErrPrefix = regexp.MustCompile(`^unexpected status code: \d+: `)

// RefreshBaseImagesCmd triggers a base image manifest refresh. By default it
// fans the request out to every server in the cluster: it asks the connected
// server for its peers (GET /api/cluster-info, which carries each node's
// api_endpoint) and POSTs the refresh endpoint to each one with the same token.
// Use --local-only to refresh just the connected server.
//
// Each server fetches the manifest itself from its configured update URL — no
// server-to-server content sync — so the whole fleet converges on the same
// (newest) catalog without gossip.
var RefreshBaseImagesCmd = &cli.Command{
	Name:  "refresh-base-images",
	Usage: "Force a base image manifest refresh across the cluster",
	Description: `Force every server to fetch the base image manifest from its update URL immediately.

Each server keeps the fetched copy only if it is newer than its active catalog. By default the command fans out to all servers in the cluster (via cluster-info); pass --local-only to refresh just the connected server. Each server must have --base-images-update-enabled on; if a server uses a manifest file it must also have --base-images-update-url set, otherwise that server reports a conflict and keeps using its file.`,
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: []string{"s"},
			Usage:   "The address of the remote server.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "API token for the remote server (must be valid cluster-wide).",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The configured server alias to use.",
			DefaultValue: "default",
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to the servers.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.BoolFlag{
			Name:  "local-only",
			Usage: "Refresh only the connected server instead of fanning out across the cluster.",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}
		token := client.GetAuthToken()
		skipVerify := cmd.GetBool("tls-skip-verify")
		localOnly := cmd.GetBool("local-only")

		// Always refresh the connected server first.
		refreshOne(ctx, labelFor(client), client)

		if localOnly {
			return nil
		}

		// Only fan out across a gossip cluster. If this server is standalone
		// (clustering off) there are no peers to reach, so the connected-server
		// refresh above is the whole job — no noise about unreachable peers.
		info, _, err := client.GetServerInfo(ctx)
		if err != nil || info == nil || !info.Clustered {
			return nil
		}

		// Enumerate the cluster and refresh each peer. Leaf nodes are not gossip
		// members and won't appear here — refresh them individually or restart
		// them (they also fetch on their own startup).
		peers, _, err := client.GetClusterInfo(ctx)
		if err != nil {
			fmt.Printf("  (couldn't enumerate cluster peers: %v; refreshed only the connected server)\n", cleanRestError(err))
			return nil
		}
		if peers == nil {
			return nil
		}

		seen := normalizeURL(client.GetBaseURL())
		for _, p := range *peers {
			ep := p.Metadata["api_endpoint"]
			if ep == "" || normalizeURL(ep) == seen {
				continue
			}
			seen += " " + normalizeURL(ep)
			peer, err := apiclient.NewClient(ep, token, skipVerify)
			if err != nil {
				fmt.Printf("  %s: skipped (%v)\n", nodeLabel(p), err)
				continue
			}
			refreshOne(ctx, nodeLabel(p), peer)
		}
		return nil
	},
}

// refreshOne calls refresh on a single server and prints a one-line result.
func refreshOne(ctx context.Context, label string, client *apiclient.ApiClient) {
	resp, _, err := client.RefreshBaseImages(ctx)
	if err != nil {
		fmt.Printf("  %s: failed: %s\n", label, cleanRestError(err))
		return
	}
	if resp.Updated {
		fmt.Printf("  %s: updated to version %s\n", label, resp.ActiveVersion)
	} else {
		fmt.Printf("  %s: already current (version %s)\n", label, resp.ActiveVersion)
	}
}

// cleanRestError strips the REST client's "unexpected status code: N: " wrapper
// so the underlying server-provided message reads cleanly.
func cleanRestError(err error) string {
	return restErrPrefix.ReplaceAllString(err.Error(), "")
}

func labelFor(client *apiclient.ApiClient) string {
	if u := client.GetBaseURL(); u != "" {
		if parsed, err := url.Parse(u); err == nil && parsed.Host != "" {
			return parsed.Host
		}
		return u
	}
	return "connected"
}

func nodeLabel(p apiclient.ClusterNodeInfo) string {
	if h := p.Metadata["hostname"]; h != "" {
		return h
	}
	return p.Address
}

func normalizeURL(u string) string {
	return strings.TrimRight(u, "/")
}
