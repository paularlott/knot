package model

import "testing"

func grants(gs ...SpaceBucketGrant) []SpaceBucketGrant { return gs }

func TestSpaceRestrictionsInactiveAllowsEverything(t *testing.T) {
	var r SpaceRestrictions
	if allowed, ro := r.BucketAccess("b1"); !allowed || ro {
		t.Fatalf("inactive restrictions: got allowed=%v readOnly=%v", allowed, ro)
	}
}

func TestSpaceRestrictionsBucketAccess(t *testing.T) {
	r := SpaceRestrictions{}.Narrow(SpaceRestrictions{
		LimitBuckets: true,
		Buckets:      grants(SpaceBucketGrant{Id: "b1", ReadOnly: true}, SpaceBucketGrant{Id: "b2"}),
	})
	cases := []struct {
		id          string
		allowed, ro bool
	}{
		{"b1", true, true},
		{"b2", true, false},
		{"b3", false, false},
	}
	for _, c := range cases {
		if allowed, ro := r.BucketAccess(c.id); allowed != c.allowed || ro != c.ro {
			t.Errorf("%s: got allowed=%v readOnly=%v, want %v %v", c.id, allowed, ro, c.allowed, c.ro)
		}
	}

	r = r.Narrow(SpaceRestrictions{Files: SpaceFilesReadOnly})
	if allowed, ro := r.BucketAccess("b2"); !allowed || !ro {
		t.Errorf("files ro: b2 should be read-only, got allowed=%v readOnly=%v", allowed, ro)
	}
	r = r.Narrow(SpaceRestrictions{Files: SpaceFilesNone})
	if allowed, _ := r.BucketAccess("b1"); allowed {
		t.Error("files none: b1 should be hidden")
	}
}

func TestSpaceRestrictionsNarrowOnlyTightens(t *testing.T) {
	r := SpaceRestrictions{}.Narrow(SpaceRestrictions{NoSecrets: true, Locked: true, Files: SpaceFilesNone})
	r = r.Narrow(SpaceRestrictions{Files: SpaceFilesReadOnly})
	if !r.Active || !r.NoSecrets || !r.Locked || r.Files != SpaceFilesNone {
		t.Fatalf("a later, looser restriction loosened them: %+v", r)
	}

	// Allowlists intersect, and read-only on either side sticks.
	r = SpaceRestrictions{}.Narrow(SpaceRestrictions{LimitBuckets: true, Buckets: grants(
		SpaceBucketGrant{Id: "b1", ReadOnly: true}, SpaceBucketGrant{Id: "b2"}, SpaceBucketGrant{Id: "b3"},
	)})
	r = r.Narrow(SpaceRestrictions{LimitBuckets: true, Buckets: grants(
		SpaceBucketGrant{Id: "b1"}, SpaceBucketGrant{Id: "b2", ReadOnly: true}, SpaceBucketGrant{Id: "b4"},
	)})
	if len(r.Buckets) != 2 {
		t.Fatalf("want b1 and b2 left, got %+v", r.Buckets)
	}
	for _, g := range r.Buckets {
		if !g.ReadOnly {
			t.Errorf("%s should be read-only", g.Id)
		}
	}

	// No allowlist on the later call keeps the earlier one.
	r = r.Narrow(SpaceRestrictions{NoSecrets: true})
	if !r.LimitBuckets || len(r.Buckets) != 2 {
		t.Fatalf("an unrelated restriction dropped the allowlist: %+v", r)
	}

	// Disjoint allowlists leave no buckets, not every bucket.
	r = r.Narrow(SpaceRestrictions{LimitBuckets: true, Buckets: grants(SpaceBucketGrant{Id: "b9"})})
	if !r.LimitBuckets || len(r.Buckets) != 0 {
		t.Fatalf("want an empty allowlist, got %+v", r)
	}
	if allowed, _ := r.BucketAccess("b1"); allowed {
		t.Error("an empty allowlist must hide every bucket")
	}
}

func TestSpaceRestrictionsNarrowDoesNotAlias(t *testing.T) {
	more := SpaceRestrictions{LimitBuckets: true, Buckets: grants(SpaceBucketGrant{Id: "b1"})}
	r := SpaceRestrictions{}.Narrow(more)
	more.Buckets[0].Id = "changed"
	if r.Buckets[0].Id != "b1" {
		t.Fatal("Narrow kept a reference to the caller's buckets")
	}
}

func TestValidSecretPrefix(t *testing.T) {
	for p, want := range map[string]bool{
		"op://Knot/": true, "op://Knot/Stripe/": true, "vault://kv/app/": true,
		"op://": false, "Knot/": false, "://Knot/": false, "o p://x/": false, "": false,
	} {
		if got := ValidSecretPrefix(p); got != want {
			t.Errorf("ValidSecretPrefix(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestSecretAllowed(t *testing.T) {
	var none SpaceRestrictions
	if !none.SecretAllowed("op://Knot/Db/password") {
		t.Fatal("inactive restrictions must allow every secret")
	}

	deny := SpaceRestrictions{}.Narrow(SpaceRestrictions{SecretsDeny: []string{"op://Knot/Stripe/"}})
	for ref, want := range map[string]bool{
		"op://Knot/Db/password":          true,
		"op://Knot/Stripe/secret key":    false,
		"op://knot/stripe/secret key":    false, // not case-sensitive
		"op://Software/CleanShot X/user": true,
	} {
		if got := deny.SecretAllowed(ref); got != want {
			t.Errorf("deny only, %s: got %v, want %v", ref, got, want)
		}
	}

	allow := SpaceRestrictions{}.Narrow(SpaceRestrictions{LimitSecrets: true, SecretsAllow: []string{"op://Knot/"}, SecretsDeny: []string{"op://Knot/Stripe/"}})
	for ref, want := range map[string]bool{
		"op://Knot/Db/password":       true,
		"op://Knot/Stripe/secret key": false, // denied within what's allowed
		"op://Software/App/password":  false, // outside the allowlist
		"vault://Knot/Db/password":    false, // another source's scheme
	} {
		if got := allow.SecretAllowed(ref); got != want {
			t.Errorf("allow and deny, %s: got %v, want %v", ref, got, want)
		}
	}

	off := allow.Narrow(SpaceRestrictions{NoSecrets: true})
	if off.SecretAllowed("op://Knot/Db/password") {
		t.Fatal("no_secrets must refuse everything")
	}
}

func TestNarrowSecrets(t *testing.T) {
	// A later, narrower allow keeps only the narrower prefix.
	r := SpaceRestrictions{}.Narrow(SpaceRestrictions{LimitSecrets: true, SecretsAllow: []string{"op://Knot/", "op://Software/"}})
	r = r.Narrow(SpaceRestrictions{LimitSecrets: true, SecretsAllow: []string{"op://Knot/Stripe/", "op://Other/"}})
	if len(r.SecretsAllow) != 1 || r.SecretsAllow[0] != "op://Knot/Stripe/" {
		t.Fatalf("allow narrowed to %v, want [op://Knot/Stripe/]", r.SecretsAllow)
	}
	// A wider allow later doesn't widen it.
	r = r.Narrow(SpaceRestrictions{LimitSecrets: true, SecretsAllow: []string{"op://"}})
	if len(r.SecretsAllow) != 1 || r.SecretsAllow[0] != "op://Knot/Stripe/" {
		t.Fatalf("a wider allow widened it to %v", r.SecretsAllow)
	}
	// Disjoint allows leave nothing allowed, not everything.
	r = r.Narrow(SpaceRestrictions{LimitSecrets: true, SecretsAllow: []string{"op://Elsewhere/"}})
	if !r.LimitSecrets || len(r.SecretsAllow) != 0 || r.SecretAllowed("op://Knot/Stripe/key") {
		t.Fatalf("disjoint allows: %+v", r)
	}
	// Denies add up, without repeats.
	r = SpaceRestrictions{}.Narrow(SpaceRestrictions{SecretsDeny: []string{"op://A/"}})
	r = r.Narrow(SpaceRestrictions{SecretsDeny: []string{"op://a/", "op://B/"}})
	if len(r.SecretsDeny) != 2 {
		t.Fatalf("deny = %v, want op://A/ and op://B/", r.SecretsDeny)
	}
	// Something unrelated keeps both lists.
	r = r.Narrow(SpaceRestrictions{Locked: true})
	if len(r.SecretsDeny) != 2 {
		t.Fatalf("an unrelated restriction dropped the deny list: %v", r.SecretsDeny)
	}
}
