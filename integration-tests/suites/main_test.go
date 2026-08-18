//go:build integration

// Package suites contains black-box integration tests that boot a real knot
// server (badgerdb + docker spaces) and drive it entirely through the API.
// Run with: task test:integration
package suites

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/cli/env"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

var (
	cfg       *harness.Config
	bins      *harness.Binaries
	server    *harness.Server
	admin     *harness.User
	user1     *harness.User
	user2     *harness.User
	templateId string
	// workhorseId is a long-lived space shared by the read-only suites
	// (files, commands, usage, logs, isolation, port-forward target);
	// booting and tearing down a space per test costs a minute each way.
	workhorseId string
	workhorseOnce sync.Once
)

// workspace returns the shared space id, booting it on first use.
func workspace(t *testing.T) string {
	t.Helper()
	workhorseOnce.Do(func() {
		harness.Progress("booting workhorse space")
		fmt.Println("== booting workhorse space ==")
		workhorseId = harness.CreateSpace(t, user1.Client, "it-wh", templateId, user1.Id)
		harness.WaitForSpaceReady(t, server, user1.Client, workhorseId)
	})
	return workhorseId
}

func TestMain(m *testing.M) {
	_ = env.Load()
	cfg = harness.LoadConfig()

	fmt.Printf("integration config: runtime=%s registry=%s image=%s container-host=%s\n",
		cfg.Runtime, cfg.Registry(), cfg.Image, cfg.ContainerHost)
	harness.Progress("loading configuration")

	if !cfg.NoBuild {
		harness.Progress("building server + agents (task build)")
		fmt.Println("== building server + agents (task build) ==")
		var err error
		bins, err = harness.BuildBinaries()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
			os.Exit(1)
		}
	}

	harness.Progress("ensuring image available: " + cfg.ImageRef())
	fmt.Println("== ensuring image available: " + cfg.ImageRef() + " ==")
	if err := harness.EnsureImageAvailable(cfg, cfg.ImageRef()); err != nil {
		fmt.Fprintf(os.Stderr, "image availability check failed: %v\n", err)
		harness.WriteReport(fmt.Sprintf("setup failed: image pull: %v", err))
		os.Exit(1)
	}

	// Interrupted runs can leave space containers behind (the server does
	// API-driven cleanup only on a clean shutdown); a leftover container
	// with the same name blocks new deployments.
	if !cfg.Keep {
		harness.PruneTestContainers("-it-")
		harness.PruneTestContainersByImage(cfg.ImageRef())
	}

	// Shared server for most suites. Feature areas that need their own
	// boot-time configuration (rate limiting, chat, totp, tunnels) boot
	// dedicated servers inside their tests.
	harness.Progress("booting default server")
	fmt.Println("== booting default server ==")
	var err error
	server, err = harness.StartServer(cfg, bins, "default", "--mcp-enabled")
	if err != nil {
		fmt.Fprintf(os.Stderr, "boot default server failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("== default server up: %s (agent listener :%d, log %s) ==\n",
		server.BaseURL, server.AgentPort, server.LogPath)

	harness.Progress("provisioning admin user")
	setupFailed := false
	admin, err = harness.ProvisionAdmin(server, "admin", "AdminPassw0rd!")
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision admin failed: %v\n", err)
		setupFailed = true
	}

	if !setupFailed {
		user1, err = harness.CreateUser(server, admin, "user1", harness.TesterPermissions())
		if err != nil {
			fmt.Fprintf(os.Stderr, "provision user1 failed: %v\n", err)
			setupFailed = true
		}
	}
	if !setupFailed {
		if harness.ProBuild {
			// The pro built-in tier is capped at two users (admin + user1),
			// so the admin acts as the second party in cross-user tests;
			// those tests assert from user1's unprivileged side.
			user2 = admin
		} else {
			user2, err = harness.CreateUser(server, admin, "user2", harness.TesterPermissions())
			if err != nil {
				fmt.Fprintf(os.Stderr, "provision user2 failed: %v\n", err)
				setupFailed = true
			}
		}
	}
	if !setupFailed {
		templateId, err = harness.CreateTemplate(server, admin.Client, "it-ubuntu", harness.TemplateOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "create template failed: %v\n", err)
			setupFailed = true
		}
	}

	code := 0
	if setupFailed {
		code = 1
		fmt.Fprintf(os.Stderr, "server log tail:\n%s\n", server.LogTail(60))
	} else {
		harness.Progress("running tests")
		fmt.Println("== running tests ==")
		code = m.Run()
	}

	harness.Progress("cleanup: waiting for background space teardown")
	fmt.Println("== waiting for background space teardown ==")
	harness.WaitForSpaceReapers(4 * time.Minute)
	cleanup()

	path, rerr := harness.WriteReport(reportHeader())
	if rerr != nil {
		fmt.Fprintf(os.Stderr, "write report failed: %v\n", rerr)
	} else {
		pass, fail, skip := harness.ReportSummary()
		fmt.Printf("\n=== integration result: %d passed, %d failed, %d skipped — report: %s ===\n",
			pass, fail, skip, path)
	}

	os.Exit(code)
}

func reportHeader() string {
	return fmt.Sprintf("Runtime: `%s`  \nImage: `%s`  \nRegistry: `%s`",
		cfg.Runtime, cfg.ImageRef(), cfg.Registry())
}

// cleanup deletes every space via the API (which tears down containers),
// stops the server and prunes any leftover test containers.
func cleanup() {
	if admin != nil && server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		if spaces, _, err := admin.Client.GetSpaces(ctx, "", true); err == nil {
			for _, s := range spaces.Spaces {
				dctx, dcancel := context.WithTimeout(context.Background(), 30*time.Second)
				admin.Client.DeleteSpace(dctx, s.Id)
				dcancel()
			}
		}
		cancel()
	}
	if server != nil {
		server.Stop()
	}
	if !cfg.Keep {
		harness.PruneTestContainers("it-")
		harness.PruneTestContainers("-it-")
	}
}

// deleteAllSpacesFor is a per-suite helper removing a user's spaces.
func deleteAllSpacesFor(t *testing.T, client *apiclient.ApiClient) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	spaces, _, err := client.GetSpaces(ctx, "", false)
	if err != nil {
		return
	}
	for _, s := range spaces.Spaces {
		harness.DeleteSpaceAndWait(t, client, s.Id)
	}
}
