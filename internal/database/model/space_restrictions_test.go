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
