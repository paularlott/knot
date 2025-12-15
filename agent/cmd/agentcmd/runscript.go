package agentcmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
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

		var scriptContent string
		var libraries map[string]string

		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}
		// Set 5-minute timeout to support AI operations with tool calling
		client.SetTimeout(5 * time.Minute)

		libraries, err = client.GetScriptLibraries(ctx)
		if err != nil {
			return fmt.Errorf("failed to get libraries: %w", err)
		}

		if _, err := os.Stat(script); err == nil {
			content, err := os.ReadFile(script)
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			scriptContent = string(content)
		} else {
			scriptObj, err := client.GetScriptByName(ctx, script)
			if err != nil {
				return fmt.Errorf("failed to get script: %w", err)
			}
			scriptContent = scriptObj.Content
		}

		user, err := client.WhoAmI(ctx)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		output, err := service.RunScript(ctx, scriptContent, args, libraries, client, user.Id)
		if err != nil {
			return fmt.Errorf("script execution failed: %w", err)
		}

		if output != "" {
			fmt.Println(output)
		}
		return nil
	},
}
