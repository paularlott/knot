package command_pool

import (
	"context"
	"fmt"
	"strconv"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var CreateCmd = &cli.Command{
	Name:        "create",
	Usage:       "Create a pool",
	Description: "Create a pool of identical spaces from a template. Set --lease-time to enable exclusive member leases (acquire / release).",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name of the new pool",
			Required: true,
		},
		&cli.StringArg{
			Name:     "template",
			Usage:    "The name or ID of the template members are created from",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:         "count",
			Usage:        "The number of spaces to keep in the pool.",
			DefaultValue: 1,
		},
		&cli.BoolFlag{
			Name:  "start",
			Usage: "Start the pool immediately.",
		},
		&cli.StringFlag{
			Name:  "lease-time",
			Usage: "Enable exclusive member leases: the maximum lease length, e.g. 30m, 2h; 'none' for leases that last until released. Leases are disabled when not set.",
		},
		&cli.StringFlag{
			Name:  "lease-extensions",
			Usage: "How many times a lease may be extended: a number, or 'unlimited'. Defaults to 0 (no extensions).",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")
		templateName := cmd.GetStringArg("template")

		count := cmd.GetInt("count")
		if count < 1 {
			return fmt.Errorf("count must be at least 1")
		}

		leaseTime, err := parseLeaseDuration(cmd.GetString("lease-time"))
		if err != nil {
			return fmt.Errorf("invalid --lease-time: %w", err)
		}

		leaseExtensions := 0
		switch value := cmd.GetString("lease-extensions"); value {
		case "":
		case "unlimited":
			leaseExtensions = -1
		default:
			leaseExtensions, err = strconv.Atoi(value)
			if err != nil || leaseExtensions < -1 {
				return fmt.Errorf("invalid --lease-extensions %q (use a number or unlimited)", value)
			}
		}
		if leaseTime == 0 && leaseExtensions != 0 {
			return fmt.Errorf("--lease-extensions needs --lease-time to enable leases")
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		// Accept a template name or ID
		templateId := templateName
		if template, err := client.GetTemplateByName(context.Background(), templateName); err == nil && template != nil {
			templateId = template.TemplateId
		}

		fmt.Printf("Creating pool %q from template %q...\n", poolName, templateName)

		response, _, err := client.CreatePool(context.Background(), &apiclient.PoolRequest{
			Name:               poolName,
			TemplateId:         templateId,
			DesiredCount:       count,
			Active:             cmd.GetBool("start"),
			LeaseMaxTime:       leaseTime,
			LeaseMaxExtensions: leaseExtensions,
		})
		if err != nil {
			return fmt.Errorf("couldn't create pool %q: %s", poolName, cmdutil.CleanAPIError(err))
		}

		state := "created stopped"
		if cmd.GetBool("start") {
			state = "started"
		}
		fmt.Printf("Pool %q %s with %d space(s), ID: %s\n", poolName, state, count, response.Id)
		if leaseTime != 0 {
			fmt.Println("Leases enabled: use 'knot pool acquire " + poolName + "' to check out a member.")
		}
		return nil
	},
}
