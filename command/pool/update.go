package command_pool

import (
	"context"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var UpdateCmd = &cli.Command{
	Name:  "update",
	Usage: "Update the custom fields given to pool members",
	Description: `Set or remove the custom field values given to every member of a pool. Members pick up the change the next time they are created or started; restart the pool (stop, then start) to apply it to all of them.

With no flags, shows the pool's current custom field values.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "custom-field",
			Usage: "Set a custom field as name=value (can be specified multiple times).",
		},
		&cli.StringSliceFlag{
			Name:  "unset-custom-field",
			Usage: "Remove a custom field, so members get the template default (can be specified multiple times).",
		},
		&cli.BoolFlag{
			Name:  "clear-custom-fields",
			Usage: "Remove every custom field before applying --custom-field.",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")

		set, err := cmdutil.ParseCustomFields(cmd.GetStringSlice("custom-field"))
		if err != nil {
			return err
		}
		unset := cmd.GetStringSlice("unset-custom-field")
		clearAll := cmd.GetBool("clear-custom-fields")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		pool, _, err := client.GetPool(context.Background(), poolName)
		if err != nil {
			return fmt.Errorf("couldn't get pool %q: %s", poolName, cmdutil.CleanAPIError(err))
		}

		if len(set) == 0 && len(unset) == 0 && !clearAll {
			printPoolCustomFields(pool.CustomFields)
			return nil
		}

		fields := []apiclient.CustomFieldValue{}
		if !clearAll {
			for _, field := range pool.CustomFields {
				if !slices.Contains(unset, field.Name) {
					fields = append(fields, field)
				}
			}
		}
		for _, field := range set {
			i := slices.IndexFunc(fields, func(f apiclient.CustomFieldValue) bool { return f.Name == field.Name })
			if i >= 0 {
				fields[i].Value = field.Value
			} else {
				fields = append(fields, field)
			}
		}

		if _, err := client.PatchPool(context.Background(), poolName, &apiclient.PoolUpdateRequest{CustomFields: &fields}); err != nil {
			return fmt.Errorf("couldn't update pool %q: %s", poolName, cmdutil.CleanAPIError(err))
		}

		fmt.Printf("Pool %q updated; members pick up the change when they are next created or started.\n", poolName)
		printPoolCustomFields(fields)
		return nil
	},
}

func printPoolCustomFields(fields []apiclient.CustomFieldValue) {
	if len(fields) == 0 {
		fmt.Println("No custom fields set; members get the template defaults.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FIELD\tVALUE")
	for _, field := range fields {
		fmt.Fprintf(w, "%s\t%s\n", field.Name, field.Value)
	}
	w.Flush()
}
