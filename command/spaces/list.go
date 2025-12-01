package command_spaces

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var ListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List the available spaces and their status",
	Description: "Lists the available spaces for the logged in user and the state of each space.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		// Get the server zone
		pingResponse, err := client.Ping(context.Background())
		if err != nil {
			fmt.Println("Error getting server info:", err)
			os.Exit(1)
		}
		zone := pingResponse.Zone

		// Get the current user
		user, err := client.WhoAmI(context.Background())
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return nil
		}

		spaces, _, err := client.GetSpaces(context.Background(), user.Id)
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return nil
		}

		data := [][]string{{"Name", "Template", "Zone", "Status", "Ports"}}
		for _, space := range spaces.Spaces {
			// Filter by zone - only show spaces in the current zone or with no zone (skip if zone is blank)
			if zone != "" && space.Zone != "" && space.Zone != zone {
				continue
			}

			status := ""
			ports := make([]string, 0)

			if space.IsRemote {
				status = "Remote "
			}

			if space.IsDeployed {
				if space.IsPending {
					status = status + "Stopping"
				} else {
					status = status + "Running"
				}
			} else if space.IsDeleting {
				status = status + "Deleting"
			} else if space.IsPending {
				status = status + "Starting"
			}

			// The list of HTTP ports
			for port, desc := range space.HttpPorts {
				var p string
				if port == desc {
					p = port
				} else {
					p = fmt.Sprintf("%s (%s)", desc, port)
				}
				ports = append(ports, p)
			}

			// The list of TCP ports
			for port, desc := range space.TcpPorts {
				var p string
				if port == desc {
					p = port
				} else {
					p = fmt.Sprintf("%s (%s)", desc, port)
				}
				ports = append(ports, p)
			}

			// Join the ports array into a comma separated string
			portText := ""
			if len(ports) > 0 {
				portText = ports[0]
				for i := 1; i < len(ports); i++ {
					portText = portText + ", " + ports[i]
				}
			}

			data = append(data, []string{space.Name, space.TemplateName, space.Zone, status, portText})
		}

		util.PrintTable(data)
		return nil
	},
}
