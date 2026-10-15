package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/util/rest"
)

// fakeReadServer answers /files/read like the server does: a ranged read
// returns the bytes in data, an old server ignores the range and returns
// the whole file as text. It replies in JSON or msgpack as asked.
func fakeReadServer(t *testing.T, file []byte, oldServer bool, reads *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(build.VersionHeader, build.Version)
		var req struct {
			Path       string `json:"path" msgpack:"path"`
			ByteOffset int64  `json:"byte_offset" msgpack:"byte_offset"`
			ByteLength int64  `json:"byte_length" msgpack:"byte_length"`
		}
		if err := rest.DecodeRequestBody(w, r, &req); err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		*reads++
		type reply struct {
			Success  bool   `json:"success" msgpack:"success"`
			Content  string `json:"content,omitempty" msgpack:"content,omitempty"`
			Ranged   bool   `json:"ranged,omitempty" msgpack:"ranged,omitempty"`
			Data     []byte `json:"data,omitempty" msgpack:"data,omitempty"`
			FileSize int64  `json:"file_size,omitempty" msgpack:"file_size,omitempty"`
		}
		if oldServer || req.ByteLength == 0 {
			rest.WriteResponse(200, w, r, reply{Success: true, Content: string(file)})
			return
		}
		start := min(req.ByteOffset, int64(len(file)))
		end := min(start+req.ByteLength, int64(len(file)))
		rest.WriteResponse(200, w, r, reply{Success: true, Ranged: true, Data: file[start:end], FileSize: int64(len(file))})
	}))
}

// binaryFile is every byte value, over several read pieces.
func binaryFile() []byte {
	file := make([]byte, 2*readPieceSize+1000)
	for i := range file {
		file[i] = byte(i)
	}
	return file
}

func TestReadSpaceFileBinary(t *testing.T) {
	file := binaryFile()
	for _, accept := range []string{rest.ContentTypeMsgPack, rest.ContentTypeJSON} {
		reads := 0
		server := fakeReadServer(t, file, false, &reads)
		c, err := NewClient(server.URL, "token", true)
		if err != nil {
			t.Fatal(err)
		}
		c.GetRESTClient().SetAccept(accept)

		got, err := c.ReadSpaceFile(context.Background(), "space", "/f.bin")
		server.Close()
		if err != nil {
			t.Fatalf("%s: ReadSpaceFile: %v", accept, err)
		}
		if !bytes.Equal([]byte(got), file) {
			t.Errorf("%s: got %d bytes, want the %d written, unchanged", accept, len(got), len(file))
		}
		if reads != 3 {
			t.Errorf("%s: %d reads, want 3 pieces", accept, reads)
		}
	}
}

func TestReadSpaceFileEmpty(t *testing.T) {
	reads := 0
	server := fakeReadServer(t, nil, false, &reads)
	defer server.Close()
	c, _ := NewClient(server.URL, "token", true)
	got, err := c.ReadSpaceFile(context.Background(), "space", "/empty")
	if err != nil || got != "" || reads != 1 {
		t.Errorf("empty file: %q, %v after %d reads", got, err, reads)
	}
}

func TestReadSpaceFileOldServer(t *testing.T) {
	reads := 0
	server := fakeReadServer(t, []byte("plain text"), true, &reads)
	defer server.Close()
	c, _ := NewClient(server.URL, "token", true)

	if _, err := c.ReadSpaceFileBytes(context.Background(), "space", "/f", 0, 10); err != ErrRangedReadUnsupported {
		t.Errorf("ReadSpaceFileBytes from an old server: err = %v, want ErrRangedReadUnsupported", err)
	}
	got, err := c.ReadSpaceFile(context.Background(), "space", "/f")
	if err != nil || got != "plain text" {
		t.Errorf("ReadSpaceFile from an old server: %q, %v, want the whole file", got, err)
	}
}

// TestRangedReadJSONCarriesBytes pins the wire format: in JSON the bytes
// travel base64-encoded, which a string field would mangle.
func TestRangedReadJSONCarriesBytes(t *testing.T) {
	encoded, _ := json.Marshal(struct {
		Data []byte `json:"data"`
	}{[]byte{0x80, 0xff, 0x00}})
	if string(encoded) != `{"data":"gP8A"}` {
		t.Errorf("data encodes as %s", encoded)
	}
}

// TestCheckServerVersion: the knot command works only with a server on its
// own major.minor version, and says so plainly otherwise.
func TestCheckServerVersion(t *testing.T) {
	mm := build.MajorMinor(build.Version)
	for _, tc := range []struct {
		name    string
		version string
		status  int
		wantErr string
	}{
		{"same version", build.Version, 200, ""},
		{"another patch", mm + ".99", 200, ""},
		{"older minor", "0.1.0", 200, "use a knot command from version 0.1"},
		{"newer major", "99.0.0", 200, "use a knot command from version 99.0"},
		{"too old to say", "", 200, "older than knot 0.41"},
		{"refused as too old to say", "", 404, "older than knot 0.41"},
		{"a proxy's error", "", 502, "unexpected status code: 502"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.version != "" {
				w.Header().Set(build.VersionHeader, tc.version)
			}
			w.WriteHeader(tc.status)
			w.Write([]byte(`{"status":true}`))
		}))
		c, _ := NewClient(server.URL, "token", true)
		_, err := c.Ping(context.Background())
		server.Close()
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: err = %v, want one saying %q", tc.name, err, tc.wantErr)
		}
		if strings.Contains(tc.name, "version") || strings.Contains(tc.name, "minor") || strings.Contains(tc.name, "major") {
			if tc.wantErr != "" && !errors.Is(err, ErrServerVersion) {
				t.Errorf("%s: err = %v, want ErrServerVersion", tc.name, err)
			}
		}
	}
}
