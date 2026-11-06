package command_files

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/syncproject"
)

var syncProjectCmd = &cli.Command{
	Name:  "sync-project",
	Usage: "Run sync projects: one or more syncs described in a TOML file",
	Description: `A sync project is one or more sync jobs, each keeping a local directory and a bucket folder in step as knot file sync does, described in a TOML file:

  name = "foo"                       # defaults to the file's name
  exclude = ["node_modules/"]        # left out of every job
  no_default_excludes = false        # true: every job syncs .git and editor scratch files too

  [[sync]]
  name     = "src"                   # defaults to the directory's name
  local    = "~/src/foo"
  remote   = "src:projects/foo/"
  snapshot = "src:templates/go/"     # made into remote when remote does not exist yet
  mode     = "two-way"               # two-way (default), download or upload
  watch    = true                    # keep syncing changes (default true)
  delete   = true                    # pass deletions on (default true)
  exclude  = [".cache/"]
  exclude_from = ".gitignore"        # more patterns, from a file in the directory
  no_default_excludes = false        # true syncs .git and editor scratch files too
  allow_mass_delete = false

On the desktop, knot file sync-project run sync.toml runs a project until interrupted. In a space, knot file sync-project start sync.toml hands it to the space's agent, which runs it in the background, and again whenever the agent starts, until stopped; list, status and stop manage it.

As knot file sync does, watched and two-way jobs leave out version control folders (.git, .hg, .svn) and editor and system scratch files; no_default_excludes, for the project or a job, syncs everything the exclude patterns do not leave out.

A job with a snapshot starts its bucket folder as a snapshot of another folder (knot file snapshot) the first time, when the folder does not exist; each start after syncs the folder as it is.`,
	Commands: []*cli.Command{
		syncProjectRunCmd,
		syncProjectStartCmd,
		syncProjectStopCmd,
		syncProjectListCmd,
		syncProjectStatusCmd,
	},
}

var syncProjectRunCmd = &cli.Command{
	Name:        "run",
	Usage:       "Run a sync project until interrupted",
	Description: "Run every job of a sync project here, until Ctrl+C; jobs that do not watch run once, and the command ends once they all have.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "file", Usage: "The project's TOML file", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		p, _, err := readProject(cmd.GetStringArg("file"))
		if err != nil {
			return err
		}
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		return runProject(ctx, func() (*apiclient.ApiClient, error) { return client, nil }, p, nil)
	},
}

// runProject runs a project in the foreground: until every job has ended,
// or until interrupted.
func runProject(ctx context.Context, client clientFunc, p *syncproject.Project, follow eventFollower) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	r := startProject(ctx, client, p, false, log.WithGroup("sync"), follow)
	ended := make(chan struct{})
	go func() {
		r.done()
		close(ended)
	}()
	select {
	case <-ended:
	case <-ctx.Done():
		r.stop()
		return nil
	}
	if r.failed() {
		return fmt.Errorf("sync project %s: a job failed", p.Name)
	}
	return nil
}

var syncProjectStartCmd = &cli.Command{
	Name:        "start",
	Usage:       "Run a sync project in the space's agent",
	Description: "Hand a sync project to the space's agent, which runs it in the background, and again whenever the agent starts, until it is stopped. A project of the same name is replaced, so a startup script can start its project on every start. Inside a space only.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "file", Usage: "The project's TOML file", Required: true},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "wait", Usage: "Wait until every job has made its first pass, failing if one failed."},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		if err := needAgent(); err != nil {
			return err
		}
		p, content, err := readProject(cmd.GetStringArg("file"))
		if err != nil {
			return err
		}
		var resp agentlink.SyncProjectResponse
		if err := agentlink.SendWithResponseMsgTimeout(agentlink.CommandSyncProjectStart, agentlink.SyncProjectStartRequest{
			Content: string(content), Name: syncproject.NameFromPath(cmd.GetStringArg("file")),
		}, &resp, 30*time.Second); err != nil {
			return agentErr(err)
		}
		if !resp.Success {
			return fmt.Errorf("%s", resp.Error)
		}
		if !cmd.GetBool("wait") {
			fmt.Printf("started sync project %s (%d jobs)\n", resp.Name, len(p.Sync))
			return nil
		}
		return waitForProject(ctx, resp.Name)
	},
}

// waitForProject waits for each job of a project the agent runs to make its
// first pass, or fail.
func waitForProject(ctx context.Context, name string) error {
	for {
		ps, err := agentProjects()
		if err != nil {
			return err
		}
		var mine *syncproject.ProjectStatus
		for i := range ps {
			if ps[i].Name == name {
				mine = &ps[i]
			}
		}
		if mine == nil {
			return fmt.Errorf("sync project %s is no longer running", name)
		}
		ready, failed := true, false
		for _, j := range mine.Jobs {
			switch {
			case j.State == "failed":
				failed = true
			case j.Passes == 0 && j.State != "done":
				ready = false
			}
		}
		if failed || ready {
			printJobs(os.Stdout, []syncproject.ProjectStatus{*mine})
			if failed {
				return fmt.Errorf("sync project %s: a job failed", name)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

var syncProjectStopCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a sync project the space's agent runs",
	Description: "Stop a sync project and forget it, so it is not started again. What it synced stays, locally and in the bucket. The template's sync project cannot be stopped. Inside a space only.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "The project's name", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		if err := needAgent(); err != nil {
			return err
		}
		var resp agentlink.SyncProjectResponse
		if err := agentlink.SendWithResponseMsgTimeout(agentlink.CommandSyncProjectStop, agentlink.SyncProjectStopRequest{Name: cmd.GetStringArg("name")}, &resp, 30*time.Second); err != nil {
			return agentErr(err)
		}
		if !resp.Success {
			return fmt.Errorf("%s", resp.Error)
		}
		fmt.Printf("stopped sync project %s\n", cmd.GetStringArg("name"))
		return nil
	},
}

var syncProjectListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List the sync projects the space's agent runs",
	Description: "List the sync projects the space's agent runs, the template's among them. Inside a space only.",
	Flags:       []cli.Flag{jsonFlag},
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		if err := needAgent(); err != nil {
			return err
		}
		ps, err := agentProjects()
		if err != nil {
			return err
		}
		if cmd.GetBool("json") {
			return printJSON(ps)
		}
		if len(ps) == 0 {
			fmt.Println("no sync projects are running")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tJOBS\tSTATE")
		for _, p := range ps {
			name := p.Name
			if p.Template {
				name += " (template)"
			}
			fmt.Fprintf(w, "%s\t%d\t%s\n", name, len(p.Jobs), projectState(p))
		}
		return w.Flush()
	},
}

var syncProjectStatusCmd = &cli.Command{
	Name:        "status",
	Usage:       "Show how the jobs of the space's sync projects are doing",
	Description: "Show each job of a sync project the space's agent runs, or of all of them: its state, passes, what it has moved and its last error. Inside a space only.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "name", Usage: "The project's name; all of them when not given"},
	},
	Flags:   []cli.Flag{jsonFlag},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		if err := needAgent(); err != nil {
			return err
		}
		ps, err := agentProjects()
		if err != nil {
			return err
		}
		if name := cmd.GetStringArg("name"); name != "" {
			var mine []syncproject.ProjectStatus
			for _, p := range ps {
				if p.Name == name {
					mine = append(mine, p)
				}
			}
			if len(mine) == 0 {
				return fmt.Errorf("no sync project %s is running", name)
			}
			ps = mine
		}
		if cmd.GetBool("json") {
			return printJSON(ps)
		}
		if len(ps) == 0 {
			fmt.Println("no sync projects are running")
			return nil
		}
		printJobs(os.Stdout, ps)
		return nil
	},
}

// readProject reads and checks a project file.
func readProject(file string) (*syncproject.Project, []byte, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, err
	}
	p, err := syncproject.Parse(data, syncproject.NameFromPath(file))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", file, err)
	}
	if err := checkProject(p); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", file, err)
	}
	return p, data, nil
}

func needAgent() error {
	if !agentlink.IsAgentRunning() {
		return fmt.Errorf("the space's agent runs sync projects, and none is running here: outside a space, use knot file sync-project run")
	}
	return nil
}

func agentErr(err error) error {
	if err == agentlink.ErrSpaceLocked {
		return fmt.Errorf("this space is locked: its processes cannot manage sync projects")
	}
	return fmt.Errorf("asking the space's agent: %w", err)
}

func agentProjects() ([]syncproject.ProjectStatus, error) {
	var resp agentlink.SyncProjectResponse
	if err := agentlink.SendWithResponseMsg(agentlink.CommandSyncProjectList, nil, &resp); err != nil {
		return nil, agentErr(err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return resp.Projects, nil
}

// projectState sums up a project's jobs' states.
func projectState(p syncproject.ProjectStatus) string {
	counts := map[string]int{}
	var order []string
	for _, j := range p.Jobs {
		if counts[j.State] == 0 {
			order = append(order, j.State)
		}
		counts[j.State]++
	}
	parts := make([]string, 0, len(order))
	for _, s := range order {
		if len(order) == 1 {
			parts = append(parts, s)
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return strings.Join(parts, ", ")
}

func printJobs(out *os.File, ps []syncproject.ProjectStatus) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tJOB\tSTATE\tLOCAL\tREMOTE\tPASSES\tLAST PASS\tUP\tDOWN\tDELETED\tCONFLICTS\tERROR")
	for _, p := range ps {
		for _, j := range p.Jobs {
			last := "-"
			if t, err := time.Parse(time.RFC3339, j.LastPass); err == nil {
				last = time.Since(t).Round(time.Second).String() + " ago"
			}
			errText := j.Error
			if errText == "" {
				errText = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%d\t%d\t%d\t%s\n", p.Name, j.Name, j.State, j.Local, j.Remote, j.Passes, last, j.Uploaded, j.Downloaded, j.Deleted, j.Conflicts, errText)
		}
	}
	w.Flush()
}
