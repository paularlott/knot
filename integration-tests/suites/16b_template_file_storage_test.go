//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/integration-tests/harness"
)

// TestTemplateDisablesFileStorage proves a template's Disable File Storage
// setting refuses file storage from its spaces, while spaces from other
// templates — and the user outside any space — keep access.
func TestTemplateDisablesFileStorage(t *testing.T) {
	harness.Feature(t, "files")
	s, adminUser := bootDedicated(t, "tplfiles", "--files-path", t.TempDir())

	ctx, cancel := testCtx(60)
	defer cancel()
	if _, err := adminUser.Client.CreateFileBucket(ctx, "it-gate"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	openTmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_files_open"), harness.TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	closedTmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_files_closed"), harness.TemplateOptions{DisableFileStorage: true})
	if err != nil {
		t.Fatal(err)
	}

	open := harness.CreateSpace(t, adminUser.Client, "it-files-open", openTmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, open)
	closed := harness.CreateSpace(t, adminUser.Client, "it-files-closed", closedTmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, closed)
	harness.WaitForSpaceReady(t, s, adminUser.Client, open)
	harness.WaitForSpaceReady(t, s, adminUser.Client, closed)

	out, _ := harness.TryRunCommand(adminUser.Client, open, 60, "sh", "-c", "/usr/local/bin/knot file ls 2>&1; echo EXIT=$?")
	if !strings.Contains(out, "it-gate") || !strings.Contains(out, "EXIT=0") {
		t.Fatalf("space from an open template: want the bucket listed, got %q", out)
	}

	out, _ = harness.TryRunCommand(adminUser.Client, closed, 60, "sh", "-c", "/usr/local/bin/knot file ls 2>&1; echo EXIT=$?")
	if !strings.Contains(out, "switched off") || strings.Contains(out, "EXIT=0") || strings.Contains(out, "it-gate") {
		t.Fatalf("space from a template with file storage off: want it refused, got %q", out)
	}

	// Outside a space the user still reaches their files.
	if _, err := adminUser.Client.GetFileBuckets(ctx, false); err != nil {
		t.Fatalf("list buckets outside a space: %v", err)
	}
}
