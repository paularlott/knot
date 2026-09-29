package database

import (
	"fmt"
	"testing"

	"github.com/paularlott/knot/internal/config"
	driver_badgerdb "github.com/paularlott/knot/internal/database/drivers/badgerdb"
	"github.com/paularlott/knot/internal/database/model"
)

// Benchmarks measure the read paths the caches sit on, against the raw
// badger driver (the fastest backend; MySQL/redis add a network hop on
// every uncached read).
func BenchmarkCachingReads(b *testing.B) {
	dir := b.TempDir()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: dir},
		Zone:     "bench",
	})

	raw := &driver_badgerdb.BadgerDbDriver{}
	if err := raw.Connect(); err != nil {
		b.Fatal(err)
	}
	cached := newCachingDriver(raw)

	// A template of realistic size.
	tmpl := &model.Template{Id: "bench-tmpl", Name: "bench", Active: true}
	for i := 0; i < 6; i++ {
		tmpl.Ports = append(tmpl.Ports, model.TemplatePort{Name: fmt.Sprintf("p%d", i), Port: uint16(8000 + i), Protocol: "tcp"})
	}
	if err := raw.SaveTemplate(tmpl, nil); err != nil {
		b.Fatal(err)
	}

	// A pool with 2 members in a database of 200 spaces — the shape the
	// old per-connection GetSpaces() scan paid for in full.
	for i := 0; i < 200; i++ {
		space := model.NewSpace(fmt.Sprintf("s%d", i), "", "u1", tmpl.Id, "bash", &[]model.AltNameEntry{}, "bench", "", nil)
		if i < 2 {
			space.PoolId = "pool-1"
		}
		space.IsDeployed = true
		if err := raw.SaveSpace(space, nil); err != nil {
			b.Fatal(err)
		}
	}

	b.Run("template/badger-direct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := raw.GetTemplate("bench-tmpl"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("template/cached-hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := cached.GetTemplate("bench-tmpl"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("pool-members/badger-scan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := raw.GetSpacesByPoolId("pool-1"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("pool-members/cached-hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := cached.GetSpacesByPoolId("pool-1"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
