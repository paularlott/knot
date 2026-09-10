package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestOptionKeysFromResult(t *testing.T) {
	cases := []struct {
		name   string
		result any
		want   []string
	}{
		{"string array", []any{"a", "b"}, []string{"a", "b"}},
		{"options array of strings", map[string]any{"options": []any{"a", "b"}}, []string{"a", "b"}},
		{
			"options of key/text pairs",
			map[string]any{"options": []any{
				map[string]any{"key": "k1", "text": "First"},
				map[string]any{"text": "Second"},
			}},
			[]string{"k1", "Second"},
		},
		{"empty result", nil, []string{}},
		{"options not an array", map[string]any{"options": "nope"}, []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := optionKeysFromResult(c.result)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

// fakeFetchKeys stands in for the plugin field handler dispatch.
type fakeFetchKeys struct {
	keys  map[string][]string
	err   map[string]error
	calls []string
}

func (f *fakeFetchKeys) fetch(_ context.Context, _ *model.User, field model.TemplateCustomField) ([]string, error) {
	f.calls = append(f.calls, field.Handler)
	if err, ok := f.err[field.Handler]; ok {
		return nil, err
	}
	return f.keys[field.Handler], nil
}

func optionTestTemplate() *model.Template {
	return &model.Template{CustomFields: []model.TemplateCustomField{
		{Name: "manual", Type: "select", Options: []string{"one", "two"}},
		{Name: "served", Type: "select", Handler: "plugin.p.list"},
		{Name: "free", Type: "text"},
		{Name: "flag", Type: "bool"},
	}}
}

func TestInvalidCustomFieldOptions(t *testing.T) {
	fake := &fakeFetchKeys{keys: map[string][]string{
		"plugin.p.list": {"alpha", "beta"},
	}}
	template := optionTestTemplate()

	values := func(pairs ...[2]string) []model.SpaceCustomField {
		out := make([]model.SpaceCustomField, 0, len(pairs))
		for _, p := range pairs {
			out = append(out, model.SpaceCustomField{Name: p[0], Value: p[1]})
		}
		return out
	}

	t.Run("valid values pass", func(t *testing.T) {
		invalid, err := invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"manual", "one"},
			[2]string{"served", "beta"},
		), nil, fake.fetch)
		if err != nil || len(invalid) != 0 {
			t.Fatalf("got invalid=%v err=%v, want none", invalid, err)
		}
	})

	t.Run("invalid manual and handler values reported together", func(t *testing.T) {
		invalid, err := invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"manual", "three"},
			[2]string{"served", "gamma"},
		), nil, fake.fetch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(invalid) != 2 {
			t.Fatalf("got %v, want two segments", invalid)
		}
		if invalid[0] != `manual (got "three"; valid: one, two)` {
			t.Errorf("manual segment = %q", invalid[0])
		}
		if invalid[1] != `served (got "gamma"; valid: alpha, beta)` {
			t.Errorf("handler segment = %q", invalid[1])
		}
	})

	t.Run("blank allowed when not required", func(t *testing.T) {
		invalid, err := invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"manual", ""},
			[2]string{"served", "  "},
		), nil, fake.fetch)
		if err != nil || len(invalid) != 0 {
			t.Fatalf("got invalid=%v err=%v, want none", invalid, err)
		}
	})

	t.Run("other field types untouched", func(t *testing.T) {
		invalid, err := invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"free", "anything"},
			[2]string{"flag", "true"},
		), nil, fake.fetch)
		if err != nil || len(invalid) != 0 {
			t.Fatalf("got invalid=%v err=%v, want none", invalid, err)
		}
	})

	t.Run("fetch failure is an error naming the field", func(t *testing.T) {
		failing := &fakeFetchKeys{err: map[string]error{"plugin.p.list": errors.New("plugin not installed")}}
		_, err := invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"served", "alpha"},
		), nil, failing.fetch)
		if err == nil {
			t.Fatal("expected an error")
		}
		want := `could not load options for custom field "served"`
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name the field", err)
		}
	})

	t.Run("unchanged values skipped on update", func(t *testing.T) {
		previous := map[string]string{"served": "gamma"} // no longer an option
		invalid, err := invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"manual", "one"},
			[2]string{"served", "gamma"},
		), previous, fake.fetch)
		if err != nil || len(invalid) != 0 {
			t.Fatalf("got invalid=%v err=%v, want none", invalid, err)
		}
	})

	t.Run("changed value validated against previous", func(t *testing.T) {
		previous := map[string]string{"served": "alpha"}
		invalid, err := invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"served", "gamma"},
		), previous, fake.fetch)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(invalid) != 1 || invalid[0] != `served (got "gamma"; valid: alpha, beta)` {
			t.Fatalf("got %v, want one served segment", invalid)
		}
	})

	t.Run("no dispatch for blank or manual fields", func(t *testing.T) {
		calls := &fakeFetchKeys{keys: fake.keys}
		_, _ = invalidCustomFieldOptions(context.Background(), nil, template, values(
			[2]string{"manual", "two"},
			[2]string{"served", ""},
		), nil, calls.fetch)
		if len(calls.calls) != 0 {
			t.Fatalf("handler dispatched for %v, want no dispatches", calls.calls)
		}
	})
}

func TestOptionKeysFromResultKeyTextFallback(t *testing.T) {
	// The select widget's optKey falls back to text when key is absent;
	// the validator must accept that fallback as a stored value's key.
	keys := optionKeysFromResult(map[string]any{"options": []any{
		map[string]any{"text": "Only Text"},
	}})
	if len(keys) != 1 || keys[0] != "Only Text" {
		t.Fatalf("got %v, want [Only Text]", keys)
	}
}

func TestDescribeMissingRequired(t *testing.T) {
	template := &model.Template{CustomFields: []model.TemplateCustomField{
		{Name: "size", Type: "select", Options: []string{"small", "large"}, Required: true},
		{Name: "env", Type: "autocomplete", Handler: "plugin.p.list", Required: true},
		{Name: "team", Required: true}, // no type: the default is text
	}}
	fake := &fakeFetchKeys{keys: map[string][]string{
		"plugin.p.list": {"dev", "prod"},
	}}

	described := describeMissingRequired(context.Background(), nil, template, []string{"size", "env", "team"}, fake.fetch)
	want := []string{
		"size (select; options: small, large)",
		"env (autocomplete; options: dev, prod)",
		"team (text)",
	}
	if len(described) != len(want) {
		t.Fatalf("got %v, want %v", described, want)
	}
	for i := range want {
		if described[i] != want[i] {
			t.Errorf("segment %d = %q, want %q", i, described[i], want[i])
		}
	}

	// A handler that cannot be reached degrades to naming it — the error
	// still fails the request, enrichment never does.
	failing := &fakeFetchKeys{err: map[string]error{"plugin.p.list": errors.New("plugin missing")}}
	described = describeMissingRequired(context.Background(), nil, template, []string{"env"}, failing.fetch)
	if len(described) != 1 || described[0] != "env (autocomplete; options from handler plugin.p.list)" {
		t.Fatalf("got %v, want handler fallback", described)
	}

	// A name with no matching field passes through untouched.
	described = describeMissingRequired(context.Background(), nil, template, []string{"ghost"}, fake.fetch)
	if len(described) != 1 || described[0] != "ghost" {
		t.Fatalf("got %v, want [ghost]", described)
	}
}

func TestFormatOptionListCap(t *testing.T) {
	keys := make([]string, 25)
	for i := range keys {
		keys[i] = fmt.Sprintf("opt%d", i)
	}

	// At or under the cap: everything listed.
	if got := formatOptionList(keys[:20]); got != strings.Join(keys[:20], ", ") {
		t.Fatalf("cap-boundary list = %q", got)
	}
	if got := formatOptionList(nil); got != "no options" {
		t.Fatalf("empty list = %q", got)
	}

	// Over the cap: first 20 plus a count of the rest.
	got := formatOptionList(keys)
	if !strings.HasPrefix(got, "opt0, opt1, opt2") || !strings.HasSuffix(got, "… (5 more)") {
		t.Fatalf("capped list = %q", got)
	}
}
