package driver_redis

import (
	"context"
	"errors"
	"time"

	"github.com/valkey-io/valkey-go"
)

// convertRedisError maps a missing-key reply to (nil, nil) so callers can treat
// "not found" as no result rather than an error.
func convertRedisError(err error) error {
	if errors.Is(err, valkey.Nil) {
		return nil
	}
	return err
}

func (db *RedisDbDriver) get(ctx context.Context, key string) (string, error) {
	return db.connection.Do(ctx, db.connection.B().Get().Key(key).Build()).ToString()
}

// dbScanString reads a key into target; a missing key surfaces the valkey.Nil
// error (callers such as node_id bootstrap depend on err != nil for absent keys).
func (db *RedisDbDriver) dbScanString(ctx context.Context, key string, target *string) error {
	v, err := db.get(ctx, key)
	if err == nil {
		*target = v
	}
	return err
}

// set stores value with a TTL in milliseconds precision; ttl <= 0 means no expiry.
func (db *RedisDbDriver) set(ctx context.Context, key string, value any, ttl time.Duration) error {
	var v string
	switch t := value.(type) {
	case []byte:
		v = string(t)
	default:
		v = t.(string)
	}
	b := db.connection.B().Set().Key(key).Value(v)
	if ttl > 0 {
		return db.connection.Do(ctx, b.PxMilliseconds(int64(ttl.Milliseconds())).Build()).Error()
	}
	return db.connection.Do(ctx, b.Build()).Error()
}

// del removes keys; multiple keys go out as one pipelined batch because the
// builder panics on cross-slot multi-key DEL (a cluster constraint).
func (db *RedisDbDriver) del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if len(keys) == 1 {
		return db.connection.Do(ctx, db.connection.B().Del().Key(keys...).Build()).Error()
	}
	cmds := make([]valkey.Completed, 0, len(keys))
	for _, k := range keys {
		cmds = append(cmds, db.connection.B().Del().Key(k).Build())
	}
	for _, resp := range db.connection.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return err
		}
	}
	return nil
}

// setNX stores value only if the key does not exist, returning false if it did.
func (db *RedisDbDriver) setNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	b := db.connection.B().Set().Key(key).Value(value).Nx()
	var err error
	if ttl > 0 {
		err = db.connection.Do(ctx, b.PxMilliseconds(int64(ttl.Milliseconds())).Build()).Error()
	} else {
		err = db.connection.Do(ctx, b.Build()).Error()
	}
	if errors.Is(err, valkey.Nil) {
		return false, nil
	}
	return err == nil, err
}

func (db *RedisDbDriver) keys(ctx context.Context, pattern string) ([]string, error) {
	return db.connection.Do(ctx, db.connection.B().Keys().Pattern(pattern).Build()).AsStrSlice()
}

// keyIterator walks the keys matching a pattern, gathered with a full SCAN
// cursor walk up front (every caller iterates the whole set anyway).
type keyIterator struct {
	keys []string
	idx  int
	err  error
}

func (it *keyIterator) Next(context.Context) bool {
	it.idx++
	return it.idx < len(it.keys)
}

func (it *keyIterator) Val() string {
	return it.keys[it.idx]
}

func (it *keyIterator) Err() error {
	return it.err
}

func (db *RedisDbDriver) scan(ctx context.Context, pattern string) *keyIterator {
	var keys []string
	var cursor uint64

	for {
		resp, err := db.connection.Do(ctx, db.connection.B().Scan().Cursor(cursor).Match(pattern).Build()).ToArray()
		if err != nil {
			return &keyIterator{err: err}
		}
		if len(resp) < 2 {
			break
		}
		next, err := resp[0].AsInt64()
		if err != nil {
			return &keyIterator{err: err}
		}
		ks, err := resp[1].AsStrSlice()
		if err != nil {
			return &keyIterator{err: err}
		}
		cursor = uint64(next)
		keys = append(keys, ks...)
		if cursor == 0 {
			break
		}
	}

	return &keyIterator{keys: keys, idx: -1}
}
