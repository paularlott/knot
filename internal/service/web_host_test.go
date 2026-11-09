package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/hostlabel"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/stdlib"
)

// namePart is the name in the host label for user, name and port.
func namePart(user, name, port string) string {
	return strings.Split(hostlabel.Label(user, name, port), "--")[1]
}

// A host too long for DNS gets a shortened name, which reaches the space,
// alt name or pool it stands for, for its owner and those it is shared
// with, and no one else.
func TestResolveWebHostName(t *testing.T) {
	f := newForwardFixture(t, "webhost")
	refreshPoolService()

	long := "Ticket123456-review-the-quarterly-report-and-sign-off-the-plan"
	space := f.space(t, f.owner, long)
	if got := ResolveWebHostName(f.owner.Id, long); got == nil || got.Id != space.Id {
		t.Fatalf("the full name did not resolve: %v", got)
	}
	part := namePart(f.owner.Username, long, "8080")
	if part == strings.ToLower(long) {
		t.Fatalf("the label was not shortened: %s", part)
	}
	if got := ResolveWebHostName(f.owner.Id, part); got == nil || got.Id != space.Id {
		t.Errorf("the shortened name %s did not resolve to the space", part)
	}
	if got := ResolveWebHostName(f.owner.Id, strings.ToUpper(part)); got == nil || got.Id != space.Id {
		t.Errorf("the shortened name in capitals did not resolve")
	}

	// Another space with the same start is told apart by the hash.
	twin := f.space(t, f.owner, long+"2")
	if got := ResolveWebHostName(f.owner.Id, namePart(f.owner.Username, long+"2", "8080")); got == nil || got.Id != twin.Id {
		t.Errorf("the second space's shortened name did not resolve to it: %v", got)
	}

	// Someone else's space is not reached, until it is shared with them.
	if got := ResolveWebHostName(f.client.Id, part); got != nil {
		t.Errorf("another user reached the space: %s", got.Name)
	}
	space.Shares = []string{f.client.Id}
	if err := database.GetInstance().SaveSpace(space, []string{"Shares"}); err != nil {
		t.Fatal(err)
	}
	if got := ResolveWebHostName(f.client.Id, part); got == nil || got.Id != space.Id {
		t.Errorf("the space shared with the user was not reached")
	}

	// An alt name.
	altName := "Another-very-long-alternative-name-for-the-service-in-the-space"
	alt := model.NewSpace("short", "", f.owner.Id, f.template.Id, "bash", &[]model.AltNameEntry{{Name: altName, Port: 8080}}, "fwd", "", nil)
	alt.IsDeployed = true
	if err := database.GetInstance().SaveSpace(alt, nil); err != nil {
		t.Fatal(err)
	}
	if got := ResolveWebHostName(f.owner.Id, namePart(f.owner.Username, altName, "8080")); got == nil || got.Id != alt.Id {
		t.Errorf("the shortened alt name did not resolve: %v", got)
	}

	// A pool's member.
	poolName := "Pool-of-workers-for-the-quarterly-report-and-sign-off-plan"
	f.pool(t, f.owner, poolName, 1)
	if got := ResolveWebHostName(f.owner.Id, namePart(f.owner.Username, poolName, "8080")); got == nil || !strings.HasPrefix(got.Name, poolName) {
		t.Errorf("the shortened pool name did not reach a member: %v", got)
	}

	// Names of the right shape that stand for nothing.
	for _, p := range []string{"ticket123456-00000000", "nothing", namePart(f.owner.Username, "Some-other-name-that-is-long-enough-to-be-shortened-in-a-host", "8080")} {
		if got := ResolveWebHostName(f.owner.Id, p); got != nil {
			t.Errorf("%s resolved to %s", p, got.Name)
		}
	}
}

// knot.server.host_label, which the MCP tools build URLs with, makes the
// labels the server does.
func TestHostLabelScript(t *testing.T) {
	env := scriptling.New()
	stdlib.RegisterAll(env)
	env.SetLibraryLoader(newKnotLibsLoader())
	cases := [][3]string{
		{"Alice", "Web-App", "8080"},
		{"bob", "Ticket123456-review-the-quarterly-report-and-sign-off-the-plan", "8080"},
		{"bob", "Ticket123456-review-the-quarterly-report-and-sign-off-the-plan", "vnc"},
		{strings.Repeat("u", 30), "A-name-of-sixty-four-characters-which-is-the-most-a-name-may-be", "65535"},
		{"carol", strings.Repeat("x", 39) + "-" + strings.Repeat("y", 24), "80"},
	}
	for _, c := range cases {
		if _, err := env.Eval(fmt.Sprintf("import knot.server as server\nresult = server.host_label(%q, %q, %q)", c[0], c[1], c[2])); err != nil {
			t.Fatal(err)
		}
		v, _ := env.GetVar("result")
		if want := hostlabel.Label(c[0], c[1], c[2]); fmt.Sprint(v) != want {
			t.Errorf("host_label(%v) = %v, want %s", c, v, want)
		}
	}
}
