package command_spaces

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var ListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List the available spaces and their status",
	Description: "Lists the available spaces for the logged in user, grouped by stack and pool.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all-zones",
			Usage: "Include spaces from all zones, not just the current server's zone",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		allZones := cmd.GetBool("all-zones")

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

		spaces, _, err := client.GetSpaces(context.Background(), user.Id, allZones)
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return nil
		}

		// Fetch pool names so we can label pool groups by name instead of ID
		poolNames := map[string]string{}
		if poolList, _, err := client.GetPools(context.Background()); err == nil && poolList != nil {
			for _, pool := range poolList.Pools {
				poolNames[pool.Id] = pool.Name
			}
		}

		// Partition into regular, stacked, and pooled
		var regular []spaceRow
		stackMap := map[string][]spaceRow{}
		poolMap := map[string][]spaceRow{}
		var stackOrder, poolOrder []string
		seenStack := map[string]bool{}
		seenPool := map[string]bool{}

		for _, space := range spaces.Spaces {
			// Filter by zone - only show spaces in the current zone or with no zone (skip if zone is blank)
			if !allZones && zone != "" && space.Zone != "" && space.Zone != zone {
				continue
			}

			row := spaceRow{
				Name:         space.Name,
				TemplateName: space.TemplateName,
				Zone:         space.Zone,
				Status:       spaceStatus(space.IsRemote, space.IsDeployed, space.IsPending, space.IsDeleting),
				Ports:        spacePorts(space.HttpPorts, space.TcpPorts),
			}

			if space.PoolId != "" {
				if !seenPool[space.PoolId] {
					seenPool[space.PoolId] = true
					poolOrder = append(poolOrder, space.PoolId)
				}
				poolMap[space.PoolId] = append(poolMap[space.PoolId], row)
			} else if space.Stack != "" {
				if !seenStack[space.Stack] {
					seenStack[space.Stack] = true
					stackOrder = append(stackOrder, space.Stack)
				}
				stackMap[space.Stack] = append(stackMap[space.Stack], row)
			} else {
				regular = append(regular, row)
			}
		}

		// Print regular spaces
		if len(regular) > 0 {
			printSpaceTable("Spaces", regular)
		}

		// Print stacks
		for _, stack := range stackOrder {
			printSpaceTable("Stack: "+stack, stackMap[stack])
		}

		// Print pools
		for _, poolID := range poolOrder {
			label := poolNames[poolID]
			if label == "" {
				label = poolID
			}
			printSpaceTable("Pool: "+label, poolMap[poolID])
		}

		// If nothing was printed
		if len(regular) == 0 && len(stackOrder) == 0 && len(poolOrder) == 0 {
			fmt.Println("No spaces found")
		}

		return nil
	},
}

type spaceRow struct {
	Name         string
	TemplateName string
	Zone         string
	Status       string
	Ports        string
}

func spaceStatus(isRemote, isDeployed, isPending, isDeleting bool) string {
	status := ""
	if isRemote {
		status = "Remote "
	}
	if isDeployed {
		if isPending {
			status += "Stopping"
		} else {
			status += "Running"
		}
	} else if isDeleting {
		status += "Deleting"
	} else if isPending {
		status += "Starting"
	}
	return status
}

func spacePorts(httpPorts, tcpPorts map[string]string) string {
	ports := make([]string, 0)
	for port, desc := range httpPorts {
		if port == desc {
			ports = append(ports, port)
		} else {
			ports = append(ports, fmt.Sprintf("%s (%s)", desc, port))
		}
	}
	for port, desc := range tcpPorts {
		if port == desc {
			ports = append(ports, port)
		} else {
			ports = append(ports, fmt.Sprintf("%s (%s)", desc, port))
		}
	}
	result := ""
	for i, p := range ports {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

func printSpaceTable(title string, rows []spaceRow) {
	fmt.Println()
	fmt.Println(title)
	data := [][]string{{"Name", "Template", "Zone", "Status", "Ports"}}
	for _, row := range rows {
		data = append(data, []string{row.Name, row.TemplateName, row.Zone, row.Status, row.Ports})
	}
	util.PrintTable(data)
}
