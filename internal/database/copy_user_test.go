package database

import (
	"reflect"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// fill sets every exported field reachable from v to a value that is not
// the zero one: slices and maps get two entries, pointers a value, and an
// interface (User.Preferences' values) nested maps and slices.
func fill(v reflect.Value, depth int) {
	if depth > 6 {
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString("s")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fill(p.Elem(), depth+1)
		v.Set(p)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 2, 2)
		for i := 0; i < 2; i++ {
			fill(s.Index(i), depth+1)
		}
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		for _, k := range []string{"a", "b"} {
			val := reflect.New(v.Type().Elem()).Elem()
			fill(val, depth+1)
			m.SetMapIndex(reflect.ValueOf(k).Convert(v.Type().Key()), val)
		}
		v.Set(m)
	case reflect.Interface:
		v.Set(reflect.ValueOf(map[string]any{"list": []any{"x", map[string]any{"n": 1.0}}, "map": map[string]any{"k": []any{"y"}}}))
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fill(v.Field(i), depth+1)
			}
		}
	}
}

// mutate changes everything reachable from v in place: what a copy shares
// with its original changes in the original too.
func mutate(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			mutate(v.Elem())
		}
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		if e := v.Elem(); e.Kind() == reflect.Map || e.Kind() == reflect.Slice || e.Kind() == reflect.Pointer {
			mutate(e)
		} else if v.CanSet() {
			v.Set(reflect.ValueOf("changed"))
		}
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			if v.CanSet() {
				v.Set(reflect.ValueOf(time.Unix(1, 0)))
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				mutate(v.Field(i))
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			mutate(v.Index(i))
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			e := v.MapIndex(k)
			if e.Kind() == reflect.Interface && !e.IsNil() {
				e = e.Elem()
			}
			if e.Kind() == reflect.Map || e.Kind() == reflect.Slice || e.Kind() == reflect.Pointer {
				mutate(e)
			}
			v.SetMapIndex(k, reflect.Zero(v.Type().Elem()))
		}
		v.SetMapIndex(reflect.ValueOf("added").Convert(v.Type().Key()), reflect.Zero(v.Type().Elem()))
	case reflect.String:
		if v.CanSet() {
			v.SetString(v.String() + "!")
		}
	case reflect.Bool:
		if v.CanSet() {
			v.SetBool(!v.Bool())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.CanSet() {
			v.SetInt(v.Int() + 1)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.CanSet() {
			v.SetUint(v.Uint() + 1)
		}
	}
}

// A copy of a user equals it and shares nothing with it, whatever fields
// the user has: one added to model.User that copyUser does not copy fails
// this.
func TestCopyUserSharesNothing(t *testing.T) {
	var orig, want model.User
	fill(reflect.ValueOf(&orig).Elem(), 0)
	fill(reflect.ValueOf(&want).Elem(), 0)

	c := copyUser(&orig)
	if !reflect.DeepEqual(*c, orig) {
		t.Fatalf("the copy differs from the user:\n got %+v\nwant %+v", *c, orig)
	}
	mutate(reflect.ValueOf(c).Elem())
	if !reflect.DeepEqual(orig, want) {
		t.Errorf("changing the copy changed the user:\n got %+v\nwant %+v", orig, want)
	}
	if reflect.DeepEqual(*c, want) {
		t.Error("mutate changed nothing: the test proves nothing")
	}

	// Nil and empty stay as they were.
	var bare model.User
	if c := copyUser(&bare); !reflect.DeepEqual(*c, bare) {
		t.Errorf("copy of an empty user: %+v", *c)
	}
	empty := model.User{Roles: []string{}, Preferences: map[string]any{}}
	if c := copyUser(&empty); c.Roles == nil || c.Preferences == nil {
		t.Error("empty slices and maps became nil")
	}
	if copyUser(nil) != nil {
		t.Error("copy of nil")
	}
}

func BenchmarkCopyUser(b *testing.B) {
	var u model.User
	fill(reflect.ValueOf(&u).Elem(), 0)
	for i := 0; i < b.N; i++ {
		copyUser(&u)
	}
}
