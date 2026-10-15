//go:build !server

package desktop

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/paularlott/cli"
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

	srv, err := nfsserver.Start(nfsserver.NewAPIBackend(client), nfsserver.Config{
		Listen: listen,
		Allow:  cmd.GetStringSlice("nfs-allow"),
	})
	if err != nil {
		return err
	}
	log.WithGroup("nfs").Info("serving file storage over NFS", "listen", srv.Addr().String(), "user", user.Username)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	// Save what clients have written but not yet committed.
	return srv.Close()
}
