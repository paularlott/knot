package rest

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/shamaton/msgpack/v3"
)

// DecodeMsgPack fully drains r and decodes the MessagePack payload into v.
//
// Do not call msgpack.UnmarshalRead on a real io.Reader: shamaton's stream
// decoder issues one Read per field and discards the byte count, so any short
// read (routine over HTTP/2 / chunked bodies) silently corrupts the decode.
func DecodeMsgPack(r io.Reader, v any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, v)
}

// EncodeMsgPack marshals v and writes the full MessagePack payload to w,
// looping the write to honour the io.Writer contract (MarshalWrite can
// short-write and truncate).
func EncodeMsgPack(w io.Writer, v any) error {
	data, err := msgpack.Marshal(v)
	if err != nil {
		return err
	}
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

// decodeBody decodes a reply by its Content-Type: msgpack, or else JSON.
func decodeBody(contentType string, r io.Reader, v any) error {
	if strings.Contains(contentType, ContentTypeMsgPack) {
		return DecodeMsgPack(r, v)
	}
	return decodeJSON(r, v)
}

// encodeBody encodes a request body in the given Content-Type.
func encodeBody(contentType string, v any) ([]byte, error) {
	if contentType == ContentTypeMsgPack {
		return msgpack.Marshal(v)
	}
	return json.Marshal(v)
}
