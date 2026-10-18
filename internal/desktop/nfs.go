//go:build !server

package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/nfsserver"
)

// NFSServerCmd serves a knot server's file storage over NFS, as the user of
// the API token it connects with. It is a client of the server, so as many
// can run as are wanted, on any machine, each as its own user.
var NFSServerCmd = &cli.Command{
	Name:  "nfs-server",
	Usage: "Serve file storage over NFS, to mount buckets as local folders",
	Description: `Serve the buckets of a knot server over NFSv3, acting as the user of the API token it connects with (--server and --token, or --alias), so they can be mounted without FUSE. The top level of the export holds the buckets; mkdir there makes a new bucket. Mount it whole at /, or a single bucket or folder by its path.

NFS has no authentication: anyone who can connect reaches every file the user can, so only loopback clients are accepted unless --nfs-allow says otherwise.

Docker or Podman volume (from a Docker Desktop or OrbStack VM, use host.docker.internal as the address):

  docker volume create --driver local --opt type=nfs \
    --opt o=addr=127.0.0.1,port=2049,mountport=2049,nfsvers=3,tcp,nolock \
    --opt device=:/ knot-files

macOS:

  mount_nfs -o vers=3,tcp,port=2049,mountport=2049,nolocks,noresvport,locallocks 127.0.0.1:/mybucket ~/mnt

Linux:

  mount -t nfs -o vers=3,tcp,port=2049,mountport=2049,nolock 127.0.0.1:/ /mnt/knot`,
	MaxArgs: cli.NoArgs,
	Flags: append(cmdutil.ClientFlags(false),
		&cli.StringFlag{
			Name:         "nfs-listen",
			Usage:        "The address to serve NFS on, or just a port for loopback.",
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_NFS_LISTEN"},
			DefaultValue: "127.0.0.1:2049",
		},
		&cli.IntFlag{
			Name:         "uid",
			Usage:        "The user id files without an owner of their own belong to (default the user running the server).",
			DefaultValue: -1,
		},
		&cli.IntFlag{
			Name:         "gid",
			Usage:        "The group id files without a group of their own belong to (default the group running the server).",
			DefaultValue: -1,
		},
		&cli.StringFlag{
			Name:    "cache-dir",
			Usage:   "Keep files read and written in this directory, so they need not be fetched again. No cache when not set.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_NFS_CACHE_DIR"},
		},
		&cli.StringFlag{
			Name:         "cache-size",
			Usage:        "The most the cache may hold, e.g. 512MB or 10GB.",
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_NFS_CACHE_SIZE"},
			DefaultValue: "1GB",
		},
		&cli.StringSliceFlag{
			Name:    "nfs-allow",
			Usage:   "Addresses or CIDRs allowed to connect (default loopback only). NFS has no authentication: anyone allowed can reach every file the user can.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_NFS_ALLOW"},
		},
	),
	Run: runNFSServer,
}

func runNFSServer(ctx context.Context, cmd *cli.Command) error {
	client, err := cmdutil.GetClient(cmd)
	if err != nil {
		return err
	}
	user, err := client.WhoAmI(ctx)
	if err != nil {
		return fmt.Errorf("couldn't reach the knot server: %w", cmdutil.CleanErr(err))
	}

	listen := cmd.GetString("nfs-listen")
	if _, err := strconv.Atoi(listen); err == nil {
		listen = net.JoinHostPort("127.0.0.1", listen)
	}
	cacheSize, err := parseSize(cmd.GetString("cache-size"))
	if err != nil {
		return fmt.Errorf("--cache-size: %w", err)
	}

	// The changes this server makes are named as its own in change
	// events, so it passes over them.
	writer := newWriterId()
	client.SetWriter(writer)

	srv, err := nfsserver.Start(nfsserver.NewAPIBackend(client), nfsserver.Config{
		Listen:    listen,
		Allow:     cmd.GetStringSlice("nfs-allow"),
		Uid:       ownerId(cmd.GetInt("uid"), os.Getuid()),
		Gid:       ownerId(cmd.GetInt("gid"), os.Getgid()),
		CacheDir:  cmd.GetString("cache-dir"),
		CacheSize: cacheSize,
		Writer:    writer,
	})
	if err != nil {
		return err
	}
	logger := log.WithGroup("nfs")
	logger.Info("serving file storage over NFS", "listen", srv.Addr().String(), "user", user.Username)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The server says when files change, here or anywhere, so what is
	// known of them can be kept until they do.
	go func() {
		err := client.FollowEvents(ctx, func(ev apiclient.Event) {
			if ev.Type != "files:changed" {
				return
			}
			var p apiclient.FilesChangedPayload
			json.Unmarshal(ev.Payload, &p)
			files := make([]nfsserver.FileChanged, len(p.Changes))
			for i, c := range p.Changes {
				files[i] = nfsserver.FileChanged{BucketId: c.BucketId, Key: c.Key, Writer: c.Writer}
			}
			srv.Changed(p.BucketIds, files)
		}, func(connected bool) {
			if connected {
				logger.Debug("live updates connected")
			} else if ctx.Err() == nil {
				logger.Debug("live updates lost: keeping what is known of files only briefly until they are back")
			}
			srv.Live(connected)
		})
		if errors.Is(err, apiclient.ErrEventsRefused) {
			logger.Warn("the server refused live updates: keeping what is known of files only briefly")
		}
	}()

	<-ctx.Done()

	// Save what clients have written but not yet committed.
	return srv.Close()
}

// ownerId is the id given, else this process's, else root's where there is
// no such thing (Windows).
func ownerId(given, own int) uint32 {
	switch {
	case given >= 0:
		return uint32(given)
	case own >= 0:
		return uint32(own)
	}
	return 0
}

// parseSize reads a size such as 1GB, 512MB or 10G; a bare number is bytes.
func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(s, u.suffix) {
			s, mult = strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), u.mult
			break
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size, e.g. 1GB", s)
	}
	return int64(n * float64(mult)), nil
}

// newWriterId is a name for this server's changes, unique to this run.
func newWriterId() string {
	var b [12]byte
	rand.Read(b[:])
	return "nfs-" + hex.EncodeToString(b[:])
}
