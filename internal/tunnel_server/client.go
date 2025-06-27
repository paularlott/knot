package tunnel_server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type TunnelType int

const (
	serverListRefreshRate = 2 * time.Second

	WebTunnel TunnelType = iota
	PortTunnel
)

type TunnelClient struct {
	serverListMutex sync.RWMutex
	serverList      map[string]*tunnelServer
	wsServerUrl     string
	serverUrl       string
	token           string
	tunnelType      TunnelType
	protocol        string
	localPort       uint16
	tunnelName      string
	spaceName       string
	spacePort       uint16
	tlsName         string
	ctx             context.Context
	cancel          context.CancelFunc
}

type TunnelOpts struct {
	Type       TunnelType // Type of tunnel
	Protocol   string     // http, https or tcp
	LocalPort  uint16     // The local port to forward to
	TunnelName string     // The name of the tunnel for web tunnels
	SpaceName  string     // The name of the space for space tunnels
	SpacePort  uint16     // The port within the space being forwarded
	TlsName    string     // The name to present to TLS ports
}

func NewTunnelClient(wsServerUrl, serverUrl, token string, opts *TunnelOpts) *TunnelClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &TunnelClient{
		serverList:  make(map[string]*tunnelServer),
		wsServerUrl: wsServerUrl,
		serverUrl:   serverUrl,
		token:       token,
		tunnelType:  opts.Type,
		protocol:    opts.Protocol,
		localPort:   opts.LocalPort,
		tunnelName:  opts.TunnelName,
		spaceName:   opts.SpaceName,
		spacePort:   opts.SpacePort,
		tlsName:     opts.TlsName,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (c *TunnelClient) ConnectAndServe() error {
	client, err := apiclient.NewClient(c.serverUrl, c.token, viper.GetBool("tls_skip_verify"))
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Get the current user
	user, err := client.WhoAmI(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}

	if c.tunnelType == WebTunnel {
		// Get the tunnel server info
		tunnelServerInfo, _, err := client.GetTunnelServerInfo(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get tunnel server info: %w", err)
		}

		log.Info().Msgf("https://%s--%s%s -> %s://localhost:%d", user.Username, c.tunnelName, tunnelServerInfo.Domain, c.protocol, c.localPort)

		// Add the tunnel servers to the list
		c.serverListMutex.Lock()
		for _, server := range tunnelServerInfo.TunnelServers {
			c.serverList[server] = newTunnelServer(c, server)
			c.serverList[server].ConnectAndServe()
		}

		if len(c.serverList) == 0 {
			c.serverList[c.wsServerUrl] = newTunnelServer(c, c.wsServerUrl)
			c.serverList[c.wsServerUrl].ConnectAndServe()
		}
		c.serverListMutex.Unlock()

		// Start a goroutine to refresh the server list periodically
		go func() {
			ticker := time.NewTicker(serverListRefreshRate)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					tunnelServerInfo, _, err := client.GetTunnelServerInfo(context.Background())
					if err != nil {
						log.Warn().Err(err).Msg("Failed to refresh tunnel server info")
						continue
					}

					// Look through the current server list and add any new servers
					c.serverListMutex.Lock()
					for _, server := range tunnelServerInfo.TunnelServers {
						if _, exists := c.serverList[server]; !exists {
							log.Debug().Msgf("Adding new tunnel server: %s", server)
							c.serverList[server] = newTunnelServer(c, server)
							c.serverList[server].ConnectAndServe()
						}
					}
					c.serverListMutex.Unlock()
				case <-c.ctx.Done():
					log.Debug().Msg("Stopping tunnel server list refresh")
					return
				}
			}
		}()
	} else {
		log.Info().Msgf("%s:%d -> localhost:%d", c.spaceName, c.spacePort, c.localPort)

		// Add the tunnel servers to the list
		c.serverListMutex.Lock()
		c.serverList[c.wsServerUrl] = newTunnelServer(c, c.wsServerUrl)
		c.serverList[c.wsServerUrl].ConnectAndServe()
		c.serverListMutex.Unlock()
	}

	return nil
}

func (c *TunnelClient) Shutdown() {
	c.serverListMutex.Lock()
	for _, server := range c.serverList {
		server.Shutdown()
	}
	c.serverList = make(map[string]*tunnelServer) // Clear the server list
	c.serverListMutex.Unlock()

	c.cancel()
}

func (c *TunnelClient) GetCtx() context.Context {
	return c.ctx
}
