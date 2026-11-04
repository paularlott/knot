//go:build integration

package suites

import (
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// methodEventServer is a stdio JSON-RPC method server in bash: it logs every
// request line and answers calls with an empty result.
const methodEventServer = `#!/bin/bash
while IFS= read -r line; do
  printf '%s\n' "$line" >> /tmp/it-method-events.log
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
  if [ -n "$id" ]; then
    printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$id"
  fi
done
`

// TestCustomEventsReachSharedMethods registers methods in user1's space with
// each kind of visibility and checks which of them custom events from other
// users reach: shared methods the sender may call, never private ones.
func TestCustomEventsReachSharedMethods(t *testing.T) {
	harness.Feature(t, "event-sinks")

	ctx, cancel := testCtx(60)
	defer cancel()

	ciGroup := uniqueName("it-ci")
	otherGroup := uniqueName("it-other")
	for _, name := range []string{ciGroup, otherGroup} {
		id, code, err := admin.Client.CreateGroup(ctx, &apiclient.GroupRequest{Name: name})
		if err != nil {
			t.Fatalf("create group %s: %v (status %d)", name, err, code)
		}
		t.Cleanup(func() {
			ctx, cancel := testCtx(15)
			admin.Client.DeleteGroup(ctx, id)
			cancel()
		})
		if name == ciGroup {
			// The admin plays CI: it holds use_methods and joins the CI group.
			setUserGroups(t, admin, []string{id})
		}
	}

	spaceId := harness.CreateSpace(t, user1.Client, uniqueName("it-qa"), templateId, user1.Id)
	harness.DeleteSpaceAsync(t, user1.Client, spaceId)
	harness.WaitForSpaceReady(t, server, user1.Client, spaceId)

	registration := `[server]
type = "stdio"
command = "/tmp/it-method-server.sh"

[[methods]]
name = "it.group"
description = "Shared with the CI group by listing it"
groups = ["` + ciGroup + `"]
events = ["custom.it.methods.*"]

[[methods]]
name = "it.other_group"
description = "Shared with a group the sender is not in"
groups = ["` + otherGroup + `"]
events = ["custom.it.methods.*"]

[[methods]]
name = "it.shared"
description = "Shared with everyone allowed to use shared methods"
scope = "shared"
events = ["custom.it.methods.*"]

[[methods]]
name = "it.private"
description = "Private to the owner"
events = ["custom.it.methods.*"]
`
	writeSpaceFile(t, spaceId, "/tmp/it-method-server.sh", methodEventServer)
	harness.RunCommand(t, user1.Client, spaceId, 15, "chmod", "+x", "/tmp/it-method-server.sh")
	writeSpaceFile(t, spaceId, "/tmp/it-methods.toml", registration)
	harness.RunCommand(t, user1.Client, spaceId, 30, "knot", "methods", "register", "/tmp/it-methods.toml")

	// A private method can't list groups: registration fails rather than
	// ignoring them, and the space keeps the methods it already has, which
	// the events below reach.
	writeSpaceFile(t, spaceId, "/tmp/it-bad-methods.toml", `[server]
type = "stdio"
command = "/tmp/it-method-server.sh"

[[methods]]
name = "it.bad"
description = "Private with groups"
scope = "private"
groups = ["`+ciGroup+`"]
`)
	if out, err := harness.TryRunCommand(user1.Client, spaceId, 30, "bash", "-c",
		"knot methods register /tmp/it-bad-methods.toml 2>&1; echo exit=$?"); err != nil || contains(out, "exit=0") {
		t.Fatalf("private method with groups registered: %s (err %v)", out, err)
	} else {
		mustContain(t, "private with groups error", out, "is private but lists groups")
	}

	emit := func(u *harness.User, eventType string) {
		t.Helper()
		ctx, cancel := testCtx(30)
		defer cancel()
		if code, err := u.Client.Do(ctx, "POST", "/api/events/emit", apiclient.EmitEventRequest{
			Type:    eventType,
			Payload: map[string]interface{}{"version": "1.2.3"},
		}, nil); err != nil {
			t.Fatalf("emit %s as %s: %v (status %d)", eventType, u.Username, err, code)
		}
	}

	// calledBy returns the local method names whose calls carried eventType.
	calledBy := func(eventType string) map[string]bool {
		out, _ := harness.TryRunCommand(user1.Client, spaceId, 15, "bash", "-c",
			"cat /tmp/it-method-events.log 2>/dev/null || true")
		got := map[string]bool{}
		for _, line := range strings.Split(out, "\n") {
			if !contains(line, `"`+eventType+`"`) {
				continue
			}
			for _, m := range []string{"it.group", "it.other_group", "it.shared", "it.private"} {
				if contains(line, `"method":"`+m+`"`) {
					got[m] = true
				}
			}
		}
		return got
	}

	expect := func(what, eventType string, want map[string]bool) {
		t.Helper()
		var got map[string]bool
		ok := waitForCond(30, func() bool {
			got = calledBy(eventType)
			for m := range want {
				if !got[m] {
					return false
				}
			}
			return true
		})
		// Give stray deliveries a moment to land before checking none did.
		time.Sleep(3 * time.Second)
		got = calledBy(eventType)
		if !ok || len(got) != len(want) {
			t.Fatalf("%s: methods called = %v, want %v", what, got, want)
		}
	}

	// The CI user reaches the method shared with its group and the one shared
	// with everyone, not the other group's or the private one.
	emit(admin, "it.methods.ci")
	expect("CI event", "custom.it.methods.ci", map[string]bool{"it.group": true, "it.shared": true})

	// The owner's own event reaches every one of its methods.
	emit(user1, "it.methods.owner")
	expect("owner event", "custom.it.methods.owner", map[string]bool{
		"it.group": true, "it.other_group": true, "it.shared": true, "it.private": true,
	})

	// Events raised inside a space, through its agent, are the space owner's:
	// processes there act as the owner. From the CI user's space, `knot event`
	// and knot.event.emit() reach the same methods as its API call.
	ciRunning := spaceFixture(t, uniqueName("it-ci-run"), admin.Id, admin.Client)
	mustContain(t, "knot event in the CI user's space",
		harness.RunCommand(t, admin.Client, ciRunning, 30, "bash", "-c", "knot event it.methods.cli && echo SENT"), "SENT")
	expect("knot event in the CI user's space", "custom.it.methods.cli", map[string]bool{"it.group": true, "it.shared": true})

	sctx, scancel := testCtx(120)
	out, exitCode, err := admin.Client.ExecuteScriptContent(sctx, ciRunning,
		"import knot.event\nknot.event.emit(\"it.methods.script\", {\"version\": \"1.2.3\"})\nprint(\"EMITTED\")\n", nil)
	scancel()
	if err != nil || exitCode != 0 || !contains(out, "EMITTED") {
		t.Fatalf("knot.event.emit in the CI user's space: err=%v exit=%d output=%s", err, exitCode, out)
	}
	expect("knot.event.emit in the CI user's space", "custom.it.methods.script", map[string]bool{"it.group": true, "it.shared": true})

	if !harness.ProBuild {
		// From the space of a user without use_methods, an event raised
		// inside it reaches none of another user's methods.
		noPermission := spaceFixture(t, uniqueName("it-u2-run"), user2.Id, user2.Client)
		mustContain(t, "knot event in a space without use_methods",
			harness.RunCommand(t, user2.Client, noPermission, 30, "bash", "-c", "knot event it.methods.cli.nopermission && echo SENT"), "SENT")
		expect("knot event in a space without use_methods", "custom.it.methods.cli.nopermission", map[string]bool{})

		// A user without use_methods reaches none of another user's methods.
		emit(user2, "it.methods.nopermission")
		expect("event without use_methods", "custom.it.methods.nopermission", map[string]bool{})

		// Nor through a space the CI user shares with them: the event
		// belongs to the CI user, but reaches other users' methods only on
		// the rights of whoever raised it.
		ctx, cancel := testCtx(30)
		defer cancel()
		ciSpace, code, err := admin.Client.CreateSpace(ctx, &apiclient.SpaceRequest{
			Name: uniqueName("it-ci-src"), TemplateId: templateId, UserId: admin.Id, Shell: "bash",
		})
		if err != nil {
			t.Fatalf("create CI space: %v (status %d)", err, code)
		}
		t.Cleanup(func() {
			ctx, cancel := testCtx(15)
			admin.Client.DeleteSpace(ctx, ciSpace)
			cancel()
		})
		if code, err := admin.Client.AddShare(ctx, ciSpace, user2.Id); err != nil {
			t.Fatalf("share CI space: %v (status %d)", err, code)
		}
		if err := user2.Client.EmitEvent(ctx, ciSpace, apiclient.EmitEventRequest{
			Type:    "it.methods.shared",
			Payload: map[string]interface{}{"version": "1.2.3"},
		}); err != nil {
			t.Fatalf("emit through shared space: %v", err)
		}
		expect("event raised through a shared space", "custom.it.methods.shared", map[string]bool{})
	}
}

// setUserGroups sets u's groups and restores the old ones when the test ends.
func setUserGroups(t *testing.T, u *harness.User, groups []string) {
	t.Helper()
	ctx, cancel := testCtx(30)
	defer cancel()
	current, err := admin.Client.GetUser(ctx, u.Id)
	if err != nil {
		t.Fatalf("get user %s: %v", u.Username, err)
	}
	update := func(groups []string) error {
		ctx, cancel := testCtx(30)
		defer cancel()
		return admin.Client.UpdateUser(ctx, u.Id, &apiclient.UpdateUserRequest{
			Username:       current.Username,
			Email:          current.Email,
			Roles:          current.Roles,
			Groups:         groups,
			Active:         current.Active,
			MaxSpaces:      current.MaxSpaces,
			ComputeUnits:   current.ComputeUnits,
			StorageUnits:   current.StorageUnits,
			MaxTunnels:     current.MaxTunnels,
			FileStorageMB:  current.FileStorageMB,
			MaxBuckets:     current.MaxBuckets,
			SSHPublicKey:   current.SSHPublicKey,
			GitHubUsername: current.GitHubUsername,
			PreferredShell: current.PreferredShell,
			Timezone:       current.Timezone,
			TOTPSecret:     current.TOTPSecret,
		})
	}
	if err := update(groups); err != nil {
		t.Fatalf("set groups for %s: %v", u.Username, err)
	}
	t.Cleanup(func() { update(current.Groups) })
}

// writeSpaceFile writes content to path inside the space.
func writeSpaceFile(t *testing.T, spaceId, path, content string) {
	t.Helper()
	ctx, cancel := testCtx(30)
	defer cancel()
	if err := user1.Client.WriteSpaceFile(ctx, spaceId, path, content); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
