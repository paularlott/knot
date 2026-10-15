package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/shamaton/msgpack/v3"
)

type wireInner struct {
	Name  string `json:"name" msgpack:"name"`
	Count int    `json:"count,omitempty" msgpack:"count,omitempty"`
}

// WireEmbedded is exported: msgpack, unlike JSON, skips an embedded
// struct whose type isn't, and the API types embed none.
type WireEmbedded struct {
	Region string `json:"region" msgpack:"region"`
}

// wireSample is tagged as the API types are: a msgpack tag matching every
// json one.
type wireSample struct {
	WireEmbedded
	Tagged     string            `json:"tagged" msgpack:"tagged"`
	Untagged   string            // both name it Untagged
	Skipped    string            `json:"-" msgpack:"-"`
	Omitted    string            `json:"omitted,omitempty" msgpack:"omitted,omitempty"`
	Bytes      []byte            `json:"bytes" msgpack:"bytes"`
	Text       string            `json:"text" msgpack:"text"`
	When       time.Time         `json:"when" msgpack:"when"`
	Inner      wireInner         `json:"inner" msgpack:"inner"`
	InnerPtr   *wireInner        `json:"inner_ptr,omitempty" msgpack:"inner_ptr,omitempty"`
	List       []wireInner       `json:"list" msgpack:"list"`
	Labels     map[string]string `json:"labels" msgpack:"labels"`
	Properties map[string]any    `json:"properties" msgpack:"properties"`
}

func sampleValue() wireSample {
	return wireSample{
		WireEmbedded: WireEmbedded{Region: "au"},
		Tagged:       "t",
		Untagged:     "u",
		Skipped:      "never sent",
		Bytes:        []byte{0x00, 0x80, 0xff},
		Text:         "caf\xe9 \xff not utf-8",
		When:         time.Date(2026, 10, 16, 1, 2, 3, 4, time.UTC),
		Inner:        wireInner{Name: "in", Count: 2},
		List:         []wireInner{{Name: "a"}, {Name: "b", Count: 1}},
		Labels:       map[string]string{"k": "v"},
		Properties:   map[string]any{"n": 5, "s": "x", "f": 1.5, "b": true},
	}
}

// keysOf lists the field names a value travels under, nested ones dotted.
func keysOf(prefix string, v any) []string {
	var out []string
	switch m := v.(type) {
	case map[string]any:
		for k, child := range m {
			out = append(out, prefix+k)
			out = append(out, keysOf(prefix+k+".", child)...)
		}
	case map[any]any:
		for k, child := range m {
			key := fmt.Sprint(k)
			out = append(out, prefix+key)
			out = append(out, keysOf(prefix+key+".", child)...)
		}
	}
	if l, ok := v.([]any); ok && len(l) > 0 {
		out = append(out, keysOf(prefix+"[].", l[0])...)
	}
	sort.Strings(out)
	return out
}

// TestMsgPackNamesFieldsLikeJSON: a type tagged as the API types are
// travels under the JSON names in msgpack, embedded structs, skipped and
// omitted fields included.
func TestMsgPackNamesFieldsLikeJSON(t *testing.T) {
	v := sampleValue()
	asJSON, _ := json.Marshal(v)
	var jsonTree any
	json.Unmarshal(asJSON, &jsonTree)

	asMsgPack, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var msgpackTree any
	if err := msgpack.Unmarshal(asMsgPack, &msgpackTree); err != nil {
		t.Fatal(err)
	}

	if got, want := keysOf("", msgpackTree), keysOf("", jsonTree); !reflect.DeepEqual(got, want) {
		t.Errorf("msgpack field names\n%v\nwant the JSON ones\n%v", got, want)
	}
}

func TestMsgPackRoundTrip(t *testing.T) {
	v := sampleValue()
	data, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var got wireSample
	if err := msgpack.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	want := v
	want.Skipped = ""
	// Numbers in an interface{} come back in the smallest msgpack type.
	want.Properties = got.Properties
	if got.Properties["s"] != "x" || got.Properties["b"] != true || got.Properties["f"] != 1.5 || fmt.Sprint(got.Properties["n"]) != "5" {
		t.Errorf("properties = %#v", got.Properties)
	}
	if !got.When.Equal(want.When) {
		t.Errorf("When = %v, want %v", got.When, want.When)
	}
	got.When, want.When = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip\n got %+v\nwant %+v", got, want)
	}
	// Strings carry bytes as they are, even when they aren't UTF-8.
	if got.Text != v.Text || !bytes.Equal(got.Bytes, v.Bytes) {
		t.Errorf("bytes changed: text %q bytes % x", got.Text, got.Bytes)
	}
}

func TestPreferredType(t *testing.T) {
	for accept, want := range map[string]string{
		"":                                      ContentTypeJSON,
		"*/*":                                   ContentTypeJSON,
		"application/json":                      ContentTypeJSON,
		"application/json, application/msgpack": ContentTypeJSON,
		"application/msgpack, application/json": ContentTypeMsgPack,
		"text/html, application/msgpack;q=0.9":  ContentTypeMsgPack,
	} {
		if got := preferredType(accept); got != want {
			t.Errorf("preferredType(%q) = %q, want %q", accept, got, want)
		}
	}
}

// TestServerSpeaksEachType sends one request in each type and asks for each
// reply type, checking the server decodes and answers in kind.
func TestServerSpeaksEachType(t *testing.T) {
	type body struct {
		Name string `json:"name" msgpack:"name"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in body
		if err := DecodeRequestBody(w, r, &in); err != nil {
			return
		}
		WriteResponse(http.StatusOK, w, r, body{Name: in.Name + "!"})
	}))
	defer server.Close()

	for _, sendAs := range []string{ContentTypeMsgPack, ContentTypeJSON} {
		for _, accept := range []string{ContentTypeMsgPack, ContentTypeJSON} {
			c, _ := NewClient(server.URL, "", true)
			c.SetContentType(sendAs)
			c.SetAccept(accept)
			var out body
			if _, err := c.Post(context.Background(), "/", body{Name: "hi"}, &out, 200); err != nil {
				t.Fatalf("send %s accept %s: %v", sendAs, accept, err)
			}
			if out.Name != "hi!" {
				t.Errorf("send %s accept %s: got %q, want hi!", sendAs, accept, out.Name)
			}
		}
	}
}

// TestResponseCheck: a client's response check vets every reply, failing
// the request when it objects.
func TestResponseCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", r.URL.Path)
		WriteResponse(http.StatusOK, w, r, map[string]string{"ok": "yes"})
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, "", true)
	c.SetResponseCheck(func(resp *http.Response) error {
		if resp.Header.Get("X-Test") == "/refuse" {
			return errRefused
		}
		return nil
	})
	var out map[string]string
	if _, err := c.Get(context.Background(), "/fine", &out); err != nil || out["ok"] != "yes" {
		t.Errorf("fine: %v, %v", out, err)
	}
	if _, err := c.Get(context.Background(), "/refuse", &out); err != errRefused {
		t.Errorf("refused: err = %v", err)
	}
	if _, err := c.Post(context.Background(), "/refuse", map[string]string{}, &out, 200); err != errRefused {
		t.Errorf("refused post: err = %v", err)
	}
}

var errRefused = errors.New("refused")
