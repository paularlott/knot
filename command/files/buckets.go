package command_files

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

var bucketCmd = &cli.Command{
	Name:  "bucket",
	Usage: "Manage buckets",
	Description: `Create, delete, share and transfer buckets.

A bucket is owned by the user who creates it and counts against their file
storage quota. Share it with users, groups or everyone, read-only or
read-write, and transfer it to another user.

Buckets are named <username>--<name>. Refer to your own buckets by their short
name and to buckets shared with you by their full name.`,
	Commands: []*cli.Command{
		bucketListCmd,
		bucketCreateCmd,
		bucketDeleteCmd,
		bucketInfoCmd,
		bucketPermissionsCmd,
		bucketShareCmd,
		bucketUnshareCmd,
		bucketTransferCmd,
	},
}

func listBuckets(ctx context.Context, client *apiclient.ApiClient, all, asJSON bool) error {
	list, err := client.GetFileBuckets(ctx, all)
	if err != nil {
		return apiError(err)
	}
	if asJSON {
		if list.Buckets == nil {
			list.Buckets = []apiclient.FileBucketInfo{}
		}
		return printJSON(list.Buckets)
	}
	if len(list.Buckets) == 0 {
		fmt.Println("No buckets found")
		return nil
	}

	table := [][]string{{"NAME", "OWNER", "ACCESS", "FILES", "SIZE", "SHARED"}}
	for _, b := range list.Buckets {
		table = append(table, []string{b.Display, b.OwnerName, yourAccess(b), fmt.Sprint(b.Count), formatBytes(b.Size), describeGrants(b.Grants)})
	}
	util.PrintTable(table)
	return nil
}

// yourAccess is the caller's own relationship to a bucket: owner, a share
// and how it is held, or manager when only file storage management reaches it.
func yourAccess(b apiclient.FileBucketInfo) string {
	switch b.Via {
	case "":
		return "manager"
	case "owner":
		return "owner"
	case "group":
		return b.Granted + " (group)"
	case "all":
		return b.Granted + " (all users)"
	}
	return b.Granted
}

func describeGrants(grants []apiclient.FileGrant) string {
	if len(grants) == 0 {
		return "-"
	}
	parts := make([]string, len(grants))
	for i, g := range grants {
		switch g.Type {
		case "all":
			parts[i] = "all users (" + g.Access + ")"
		default:
			parts[i] = g.Type + ":" + g.Name + " (" + g.Access + ")"
		}
	}
	return strings.Join(parts, ", ")
}

var bucketListCmd = &cli.Command{
	Name:  "list",
	Usage: "List buckets you own or that are shared with you",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "all", Usage: "List every bucket (file administrators only)."},
		jsonFlag,
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		return listBuckets(ctx, client, cmd.GetBool("all"), cmd.GetBool("json"))
	},
}

var bucketCreateCmd = &cli.Command{
	Name:        "create",
	Usage:       "Create a bucket",
	Description: "Give the short name, 3-30 lowercase letters, digits and hyphens without --. The bucket is created as <username>--<name>; refer to it by the short name.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "Bucket name", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		b, err := client.CreateFileBucket(ctx, cmd.GetStringArg("name"))
		if err != nil {
			return apiError(err)
		}
		fmt.Printf("Bucket %s created (full name %s)\n", b.Display, b.Name)
		return nil
	},
}

var bucketDeleteCmd = &cli.Command{
	Name:  "delete",
	Usage: "Delete a bucket",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "Bucket name", Required: true},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Delete the bucket even if it holds files, deleting them too."},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		name := cmd.GetStringArg("name")
		if err := client.DeleteFileBucket(ctx, name, cmd.GetBool("force")); err != nil {
			msg := cmdutil.CleanAPIError(err)
			if strings.Contains(msg, "not empty") {
				msg += " (use --force to delete it and its files)"
			}
			return fmt.Errorf("%s", msg)
		}
		fmt.Printf("Bucket %s deleted\n", name)
		return nil
	},
}

var bucketInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "Show a bucket's owner, usage and sharing",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "Bucket name", Required: true},
	},
	Flags:   []cli.Flag{jsonFlag},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		b, err := client.GetFileBucket(ctx, cmd.GetStringArg("name"))
		if err != nil {
			return apiError(err)
		}
		if cmd.GetBool("json") {
			return printJSON(b)
		}
		printBucket(b)
		return nil
	},
}

var bucketPermissionsCmd = &cli.Command{
	Name:        "permissions",
	Usage:       "List who can access a bucket",
	Description: "Lists the owner and every user, group or all-users grant with its access. Only the owner and file storage managers see the grants; others see the owner and their own access.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "Bucket name", Required: true},
	},
	Flags:   []cli.Flag{jsonFlag},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		b, err := client.GetFileBucket(ctx, cmd.GetStringArg("name"))
		if err != nil {
			return apiError(err)
		}
		if cmd.GetBool("json") {
			grants := b.Grants
			if grants == nil {
				grants = []apiclient.FileGrant{}
			}
			return printJSON(struct {
				Bucket string                `json:"bucket"`
				Owner  string                `json:"owner"`
				Access string                `json:"access"`
				Grants []apiclient.FileGrant `json:"grants"`
			}{b.Name, b.OwnerName, b.Access, grants})
		}

		table := [][]string{{"TYPE", "NAME", "ACCESS"}, {"owner", b.OwnerName, "owner"}}
		if b.Access != "owner" {
			table = append(table, []string{"you", "", b.Access})
		}
		for _, g := range b.Grants {
			name := g.Name
			if g.Type == "all" {
				name = "all users"
			}
			table = append(table, []string{g.Type, name, g.Access})
		}
		util.PrintTable(table)
		return nil
	},
}

// printSharing reports who a bucket is shared with after a share changes.
func printSharing(b *apiclient.FileBucketInfo) {
	if len(b.Grants) == 0 {
		fmt.Printf("Bucket %s is no longer shared\n", b.Display)
		return
	}
	fmt.Printf("Bucket %s now shared with: %s\n", b.Display, describeGrants(b.Grants))
}

func printBucket(b *apiclient.FileBucketInfo) {
	fmt.Printf("Name:     %s\n", b.Display)
	if b.Name != b.Display {
		fmt.Printf("Full:     %s\n", b.Name)
	}
	fmt.Printf("Owner:    %s\n", b.OwnerName)
	fmt.Printf("Access:   %s\n", yourAccess(*b))
	fmt.Printf("Files:    %d\n", b.Count)
	fmt.Printf("Size:     %s\n", formatBytes(b.Size))
	fmt.Printf("Created:  %s\n", b.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Printf("Shared:   %s\n", describeGrants(b.Grants))
}

// sharePrincipal reads --user, --group or --all.
func sharePrincipal(cmd *cli.Command) (string, string, error) {
	user, group, all := cmd.GetString("user"), cmd.GetString("group"), cmd.GetBool("all")
	n := 0
	for _, set := range []bool{user != "", group != "", all} {
		if set {
			n++
		}
	}
	if n != 1 {
		return "", "", fmt.Errorf("give exactly one of --user, --group or --all")
	}
	switch {
	case user != "":
		return "user", user, nil
	case group != "":
		return "group", group, nil
	}
	return "all", "", nil
}

var principalFlags = []cli.Flag{
	&cli.StringFlag{Name: "user", Aliases: []string{"u"}, Usage: "Username or email of the user."},
	&cli.StringFlag{Name: "group", Aliases: []string{"g"}, Usage: "Name of the group."},
	&cli.BoolFlag{Name: "all", Usage: "All users."},
}

var bucketShareCmd = &cli.Command{
	Name:        "share",
	Usage:       "Share a bucket with a user, a group or all users",
	Description: "Grants read-only access unless --write is given. Sharing again with the same user or group replaces the previous access.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "Bucket name", Required: true},
	},
	Flags: append(append([]cli.Flag{}, principalFlags...),
		&cli.BoolFlag{Name: "write", Aliases: []string{"w"}, Usage: "Allow adding, changing and deleting files."},
	),
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		grantType, name, err := sharePrincipal(cmd)
		if err != nil {
			return err
		}
		access := "read"
		if cmd.GetBool("write") {
			access = "write"
		}
		b, err := client.ShareFileBucket(ctx, cmd.GetStringArg("name"), apiclient.FileShareRequest{Type: grantType, Name: name, Access: access})
		if err != nil {
			return apiError(err)
		}
		printSharing(b)
		return nil
	},
}

var bucketUnshareCmd = &cli.Command{
	Name:  "unshare",
	Usage: "Stop sharing a bucket with a user, a group or all users",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "Bucket name", Required: true},
	},
	Flags:   principalFlags,
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		grantType, name, err := sharePrincipal(cmd)
		if err != nil {
			return err
		}
		b, err := client.UnshareFileBucket(ctx, cmd.GetStringArg("name"), apiclient.FileUnshareRequest{Type: grantType, Name: name})
		if err != nil {
			return apiError(err)
		}
		printSharing(b)
		return nil
	},
}

var bucketTransferCmd = &cli.Command{
	Name:        "transfer",
	Usage:       "Transfer a bucket to another user",
	Description: "The bucket moves into the new owner's namespace and is renamed <newuser>--<name>, so update anything that refers to it. Its content then counts against the new owner's quota; the transfer is refused if it does not fit unless an administrator gives --force.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "Bucket name", Required: true},
		&cli.StringArg{Name: "user", Usage: "Username or email of the new owner", Required: true},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Transfer even if it takes the new owner over their quota (administrators only)."},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		b, err := client.TransferFileBucket(ctx, cmd.GetStringArg("name"), cmd.GetStringArg("user"), cmd.GetBool("force"))
		if err != nil {
			return apiError(err)
		}
		fmt.Printf("Bucket now owned by %s as %s\n", b.OwnerName, b.Name)
		return nil
	},
}
