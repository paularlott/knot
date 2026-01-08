package agentcmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/service"
)

var RunScriptCmd = &cli.Command{
	Name:        "run-script",
	Usage:       "Run a scriptling script",
	Description: "Execute a scriptling script from disk or by name from the server.",
	MaxArgs:     cli.UnlimitedArgs,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "script",
			Usage:    "Script name or file path",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "Server alias to use for fetching scripts.",
			DefaultValue: "default",
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		script := cmd.GetStringArg("script")
		args := cmd.GetArgs()
		alias := cmd.GetString("alias")

		var scriptContent string
		var cfg *config.ServerAddr
		var client *apiclient.ApiClient

		// Helper to get/refresh token and create client
		getClient := func() (*apiclient.ApiClient, error) {
			// Try to get existing config
			if cmd.HasFlag("server") && cmd.HasFlag("token") {
				cfg = &config.ServerAddr{
					HttpServer: cmd.GetString("server"),
					ApiToken:   cmd.GetString("token"),
				}
			} else {
				cfg = &config.ServerAddr{}
				v, exists := cmd.ConfigFile.GetValue("client.connection." + alias + ".server")
				if exists {
					cfg.HttpServer = v.(string)
				}
				v, exists = cmd.ConfigFile.GetValue("client.connection." + alias + ".token")
				if exists {
					cfg.ApiToken = v.(string)
				}
			}

			// If no API token, try to get one from agent
			if cfg.ApiToken == "" && agentlink.IsAgentRunning() {
				var connectResp agentlink.ConnectResponse
				if err := agentlink.SendWithResponseMsg(agentlink.CommandConnect, nil, &connectResp); err == nil && connectResp.Success {
					cfg.HttpServer = connectResp.Server
					cfg.ApiToken = connectResp.Token
					cmd.ConfigFile.SetValue("client.connection."+alias+".server", cfg.HttpServer)
					cmd.ConfigFile.SetValue("client.connection."+alias+".token", cfg.ApiToken)
					cmd.ConfigFile.Save()
				}
			}

			if cfg.HttpServer == "" {
				return nil, fmt.Errorf("no server configured and agent not running")
			}
			if cfg.ApiToken == "" {
				return nil, fmt.Errorf("no API token available")
			}

			c, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
			if err != nil {
				return nil, err
			}
			c.SetTimeout(5 * time.Minute)
			return c, nil
		}

		// Get initial client
		client, err := getClient()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		// Validate token and get user - if expired and agent running, refresh and retry
		user, err := client.WhoAmI(ctx)
		if err != nil && agentlink.IsAgentRunning() {
			cfg.ApiToken = "" // Force refresh
			client, err = getClient()
			if err != nil {
				return fmt.Errorf("failed to refresh API client: %w", err)
			}
			user, err = client.WhoAmI(ctx)
			if err != nil {
				return fmt.Errorf("failed to get user: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		// Check if file exists locally
		if _, err := os.Stat(script); err == nil {
			content, err := os.ReadFile(script)
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			scriptContent = string(content)
		} else {
			scriptContent, err = client.GetScriptByName(ctx, script)
			if err != nil {
				return fmt.Errorf("failed to get script: %w", err)
			}
		}

		output, err := service.RunScript(ctx, scriptContent, args, client, user.Id)
		if err != nil {
			return fmt.Errorf("script execution failed: %w", err)
		}

		if output != "" {
			fmt.Println(output)
		}
		return nil
	},
}
