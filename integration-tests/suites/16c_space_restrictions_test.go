//go:build integration

package suites

import (
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestSpaceRestrictions proves a space can restrict its own credential from
// inside: only the buckets it keeps are visible, read-only buckets refuse
// writes, the restrictions only tighten from inside and confine the space,
// its owner can lift them from outside, and restarting the space clears them.
func TestSpaceRestrictions(t *testing.T) {
	harness.Feature(t, "files")
	s, adminUser := bootDedicated(t, "restrict", "--files-path", t.TempDir())

	ctx, cancel := testCtx(60)
	defer cancel()
	for _, name := range []string{"it-notes", "it-scratch", "it-private"} {
		if _, err := adminUser.Client.CreateFileBucket(ctx, name); err != nil {
			t.Fatalf("create bucket %s: %v", name, err)
		}
		if _, err := adminUser.Client.PutFileObject(ctx, name, "a.txt", strings.NewReader("hello"), 5, "text/plain", time.Now()); err != nil {
			t.Fatalf("put %s/a.txt: %v", name, err)
		}
	}

	tmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_restrict"), harness.TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	space := harness.CreateSpace(t, adminUser.Client, "it-restrict", tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, space)
	harness.WaitForSpaceReady(t, s, adminUser.Client, space)

	run := func(script string) string {
		out, _ := harness.TryRunCommand(adminUser.Client, space, 60, "sh", "-c", script+" 2>&1; echo EXIT=$?")
		return out
	}

	out := run("/usr/local/bin/knot agent restrict --bucket it-notes:ro --bucket it-scratch --no-secrets")
	if !strings.Contains(out, "EXIT=0") || !strings.Contains(out, "it-notes (read-only)") {
		t.Fatalf("restrict from inside the space: got %q", out)
	}

	// Only the kept buckets are visible.
	out = run("/usr/local/bin/knot file bucket list")
	if !strings.Contains(out, "it-notes") || !strings.Contains(out, "it-scratch") || strings.Contains(out, "it-private") {
		t.Fatalf("bucket list: want it-notes and it-scratch only, got %q", out)
	}
	if out = run("/usr/local/bin/knot file cat it-private:a.txt"); strings.Contains(out, "EXIT=0") {
		t.Fatalf("reading a dropped bucket worked: %q", out)
	}

	// Read-only refuses writes; read-write takes them.
	if out = run("/usr/local/bin/knot file cat it-notes:a.txt"); !strings.Contains(out, "hello") {
		t.Fatalf("reading a read-only bucket: got %q", out)
	}
	if out = run("echo x | /usr/local/bin/knot file copy - it-notes:b.txt"); strings.Contains(out, "EXIT=0") {
		t.Fatalf("writing a read-only bucket worked: %q", out)
	}
	if out = run("echo x | /usr/local/bin/knot file copy - it-scratch:b.txt"); !strings.Contains(out, "EXIT=0") {
		t.Fatalf("writing a read-write bucket: got %q", out)
	}

	// From inside, restrictions only tighten.
	if out = run("/usr/local/bin/knot agent restrict --bucket it-private"); !strings.Contains(out, "no bucket named") {
		t.Fatalf("adding back a dropped bucket: want it unknown, got %q", out)
	}
	out = run("/usr/local/bin/knot agent restrict --bucket it-notes --bucket it-scratch")
	if !strings.Contains(out, "EXIT=0") || !strings.Contains(out, "it-notes (read-only)") {
		t.Fatalf("asking for read-write on a read-only bucket must keep it read-only, got %q", out)
	}

	// Scripts see the restrictions, can't lift them, and are confined: they
	// can't change other spaces.
	probe := `import knot.space

print("RESTRICTED:" + str(knot.space.restrictions()["restricted"]))
try:
    knot.space.clear_restrictions(knot.space._this_space())
    print("CLEARED")
except Exception as e:
    print("CLEAR-REFUSED")
try:
    knot.space.set_field("00000000-0000-4000-8000-000000000000", "x", "y")
    print("REACHED")
except Exception as e:
    print("CONFINED:" + str(e))
`
	if err := adminUser.Client.WriteSpaceFile(ctx, space, "/tmp/restrict_probe.py", probe); err != nil {
		t.Fatalf("write probe script: %v", err)
	}
	out = run("/usr/local/bin/knot run-script /tmp/restrict_probe.py")
	if !strings.Contains(out, "RESTRICTED:True") || !strings.Contains(out, "CLEAR-REFUSED") || !strings.Contains(out, "CONFINED:") || !strings.Contains(out, "restricted") {
		t.Fatalf("script inside a restricted space: got %q", out)
	}

	// Its owner sees the restrictions, and can lift them from outside.
	res, _, err := adminUser.Client.GetSpaceRestrictions(ctx, space)
	if err != nil {
		t.Fatalf("get restrictions: %v", err)
	}
	if !res.Restricted || !res.NoSecrets || !res.LimitBuckets || len(res.Buckets) != 2 {
		t.Fatalf("restrictions seen from outside: %+v", res)
	}
	if res, _, err = adminUser.Client.ClearSpaceRestrictions(ctx, space); err != nil || res.Restricted {
		t.Fatalf("lift restrictions from outside: %+v, %v", res, err)
	}
	if out = run("/usr/local/bin/knot file bucket list"); !strings.Contains(out, "it-private") {
		t.Fatalf("after lifting: want every bucket, got %q", out)
	}

	// Restarting the space clears them.
	if out = run("/usr/local/bin/knot agent restrict --no-files"); !strings.Contains(out, "EXIT=0") {
		t.Fatalf("restrict again: %q", out)
	}
	if _, err := adminUser.Client.RestartSpace(ctx, space); err != nil {
		t.Fatalf("restart: %v", err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		res, _, err := adminUser.Client.GetSpaceRestrictions(ctx, space)
		if err == nil && !res.Restricted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("restrictions still in place after a restart: %+v, %v", res, err)
		}
		time.Sleep(2 * time.Second)
	}
}

// TestSpaceNoCli proves knot agent restrict --no-cli locks a space's
// processes out of knot — only showing the restrictions and restarting the
// space still work from inside — while what's started from outside the
// space keeps working: scripts run in it, with the knot API, and port
// forwards added to it.
func TestSpaceNoCli(t *testing.T) {
	harness.Feature(t, "files")
	s, adminUser := bootDedicated(t, "nocli", "--files-path", t.TempDir())

	ctx, cancel := testCtx(120)
	defer cancel()
	if _, err := adminUser.Client.CreateFileBucket(ctx, "it-locked"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	tmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_nocli"), harness.TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	space := harness.CreateSpace(t, adminUser.Client, "it-nocli", tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, space)
	target := harness.CreateSpace(t, adminUser.Client, "it-nocli-target", tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, target)
	harness.WaitForSpaceReady(t, s, adminUser.Client, space)
	harness.WaitForSpaceReady(t, s, adminUser.Client, target)

	run := func(script string) string {
		out, _ := harness.TryRunCommand(adminUser.Client, space, 60, "sh", "-c", script+" 2>&1; echo EXIT=$?")
		return out
	}

	out := run("/usr/local/bin/knot agent restrict --no-cli")
	if !strings.Contains(out, "EXIT=0") || !strings.Contains(out, "restart and shut down only") {
		t.Fatalf("restrict --no-cli: got %q", out)
	}

	// Showing the restrictions still works; everything else is refused.
	if out = run("/usr/local/bin/knot agent restrictions"); !strings.Contains(out, "EXIT=0") || !strings.Contains(out, "restart and shut down only") {
		t.Fatalf("restrictions after --no-cli: got %q", out)
	}
	for _, cmd := range []string{
		"/usr/local/bin/knot file bucket list",
		"/usr/local/bin/knot agent set-field x y",
		"/usr/local/bin/knot agent restrict --no-secrets",
		"/usr/local/bin/knot port forward 8023 it-nocli-target 2222",
	} {
		if out = run(cmd); strings.Contains(out, "EXIT=0") || !strings.Contains(out, "restricted") {
			t.Fatalf("%s: want it refused, got %q", cmd, out)
		}
	}

	// A script run in the space from outside still uses the knot API.
	probe := `import knot.space

print("NAME:" + knot.space.get(knot.space._this_space())["name"])
print("NOCLI:" + str(knot.space.restrictions()["no_cli"]))
`
	result, _, err := adminUser.Client.ExecuteScriptContent(ctx, space, probe, nil)
	if err != nil || !strings.Contains(result, "NAME:it-nocli") || !strings.Contains(result, "NOCLI:True") {
		t.Fatalf("script run from outside: %q, %v", result, err)
	}

	// A port forward added from outside still works: the target's sshd
	// answers through it.
	if code, err := adminUser.Client.ForwardPort(ctx, space, &apiclient.PortForwardRequest{
		LocalPort:  8022,
		Space:      "it-nocli-target",
		RemotePort: 2222,
	}); err != nil {
		t.Fatalf("forward port from outside: %v (status %d)", err, code)
	}
	waitFor(t, 60, func() bool {
		out := run("timeout 10 bash -c 'exec 3<>/dev/tcp/127.0.0.1/8022; head -c 16 <&3' || true")
		return strings.Contains(out, "SSH")
	})

	// Restarting from inside still works, and clears the restrictions.
	if out = run("/usr/local/bin/knot agent restart"); !strings.Contains(out, "EXIT=0") {
		t.Fatalf("restart from inside: got %q", out)
	}
	waitFor(t, 180, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		res, _, err := adminUser.Client.GetSpaceRestrictions(ctx, space)
		return err == nil && !res.Restricted
	})
}
