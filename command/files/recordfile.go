package command_files

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"
)

// A record file keeps a map, by path, on disk, so that a change to a few
// entries costs a few small writes rather than a rewrite of the whole map:
// the checksum cache and the two-way state each hold an entry for every file
// synced, which for a large tree is tens of megabytes. Changes are appended
// as records, a later record for a path replacing an earlier one. The
// records replaced are garbage: once the garbage outweighs what the file
// holds, it is written afresh, so the file is never much more than twice its
// live size and nothing is written much more than twice.
//
// The file never leaves the machine, so it is plain binary, little-endian:
//
//	header: magic (4 bytes), version (1 byte),
//	        identity length (uvarint), identity,
//	        CRC-32C of all the header before it (4 bytes)
//	record: body length (4 bytes), body,
//	        CRC-32C of the length and body (4 bytes)
//	body:   kind (1 byte: set or delete), path length (uvarint), path,
//	        then for a set, the entry as its codec writes it
//
// Each record carries its own check, so damage loses only the records from
// the damaged one on: reading stops there, keeping those before it, and the
// next save writes the file afresh. A record cut short by a crash part way
// through an append is the same.
//
// Nothing is synced to disk (fsync): a crash of the sync itself loses
// nothing written, as the system still holds it, and what a crash of the
// whole machine or a power cut loses is only the last changes, or the file
// left empty or zeroed, which is read as damage. Losing them costs no more
// than damage does (see syncstate.go), where syncing to disk at every pass
// would make each one wait on the drive and keep the system from gathering
// many small appends into one write.
//
// The identity says what the file is of (the
// directory, or the bucket folder and directory of a two-way sync).

const (
	recSet    byte = 1
	recDelete byte = 2

	// recMax is the largest record body read: anything larger is damage.
	recMax = 1 << 20
	// recMinGarbage is the garbage always allowed, so a small file is not
	// written afresh for every few changes.
	recMinGarbage = 64 << 10
)

var crcTable = crc32.MakeTable(crc32.Castagnoli)

var (
	// errRecordForeign is a file that is not a record file of this kind:
	// it is left alone rather than overwritten.
	errRecordForeign = errors.New("not a knot sync file")
	// errRecordVersion is a record file of another version of knot.
	errRecordVersion = errors.New("written by another version of knot")
)

// recordCodec writes and reads the entries of a record file.
type recordCodec[T comparable] struct {
	put func(dst []byte, path string, v T) []byte
	get func(r *recReader, path string) T
}

type recordFile[T comparable] struct {
	path     string
	magic    string
	version  byte
	identity string
	codec    recordCodec[T]

	mu      sync.Mutex
	held    map[string]T // what the file holds, with what is pending
	pending []byte       // records not yet written
	size    int64        // bytes of records in the file, with those pending
	live    int64        // bytes the entries held take as records
	written int64        // bytes of the file as last written or read
	afresh  bool         // the file is to be written afresh at the next save
	// Lost is set by load when damage cost records.
	lost bool
	// otherIdentity is the identity of a file of something else.
	otherIdentity string
}

func newRecordFile[T comparable](path, magic string, version byte, identity string, codec recordCodec[T]) *recordFile[T] {
	return &recordFile[T]{path: path, magic: magic, version: version, identity: identity, codec: codec, held: map[string]T{}}
}

func (f *recordFile[T]) header() []byte {
	b := append([]byte(f.magic), f.version)
	b = binary.AppendUvarint(b, uint64(len(f.identity)))
	b = append(b, f.identity...)
	return binary.LittleEndian.AppendUint32(b, crc32.Checksum(b, crcTable))
}

// load reads the file, returning what it holds. A missing or empty file
// holds nothing. A file of another kind is errRecordForeign; one of this
// kind but another identity holds nothing for this one, its identity in
// otherIdentity, and is written afresh at the next save; errRecordVersion
// likewise. Damage keeps what was read before it, and sets lost.
func (f *recordFile[T]) load() (map[string]T, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held, f.pending, f.size, f.live, f.written, f.lost = map[string]T{}, nil, 0, 0, 0, false
	f.afresh = true
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) || (err == nil && len(data) == 0) {
		return f.held, nil
	}
	if err != nil {
		return f.held, err
	}
	if len(data) < len(f.magic) || string(data[:len(f.magic)]) != f.magic {
		if allZero(data[:min(len(data), len(f.magic))]) {
			// What a power cut can leave of a file written just before.
			f.lost = true
			return f.held, nil
		}
		return f.held, errRecordForeign
	}
	r := &recReader{b: data[len(f.magic):]}
	version := r.byte()
	identity := r.str()
	headerLen := len(data) - len(r.b)
	crc := r.uint32()
	if r.bad || crc != crc32.Checksum(data[:headerLen], crcTable) {
		f.lost = true // even the header: nothing kept
		return f.held, nil
	}
	if version != f.version {
		return f.held, errRecordVersion
	}
	if identity != f.identity {
		f.otherIdentity = identity
		return f.held, nil
	}
	off := headerLen + 4
	for off < len(data) {
		n, ok := f.readRecord(data[off:])
		if !ok {
			f.lost = true
			break
		}
		off += n
	}
	f.written = int64(off)
	f.afresh = f.lost
	for p, v := range f.held {
		f.live += int64(len(f.record(nil, recSet, p, v)))
	}
	return f.held, nil
}

// readRecord applies the record at the start of b, returning its length.
func (f *recordFile[T]) readRecord(b []byte) (int, bool) {
	if len(b) < 8 {
		return 0, false
	}
	n := binary.LittleEndian.Uint32(b)
	if n == 0 || n > recMax || int(n)+8 > len(b) {
		return 0, false
	}
	end := 4 + int(n)
	if binary.LittleEndian.Uint32(b[end:]) != crc32.Checksum(b[:end], crcTable) {
		return 0, false
	}
	r := &recReader{b: b[4:end]}
	kind := r.byte()
	p := r.str()
	switch kind {
	case recSet:
		v := f.codec.get(r, p)
		if r.bad || len(r.b) != 0 {
			return 0, false
		}
		f.held[p] = v
	case recDelete:
		if r.bad || len(r.b) != 0 {
			return 0, false
		}
		delete(f.held, p)
	default:
		return 0, false
	}
	f.size += int64(end + 4)
	return end + 4, true
}

// record appends to dst a record of kind for path.
func (f *recordFile[T]) record(dst []byte, kind byte, path string, v T) []byte {
	start := len(dst)
	dst = append(dst, 0, 0, 0, 0, kind)
	dst = binary.AppendUvarint(dst, uint64(len(path)))
	dst = append(dst, path...)
	if kind == recSet {
		dst = f.codec.put(dst, path, v)
	}
	binary.LittleEndian.PutUint32(dst[start:], uint32(len(dst)-start-4))
	return binary.LittleEndian.AppendUint32(dst, crc32.Checksum(dst[start:], crcTable))
}

// update brings what the file holds to want, as records pending for the next
// save: each lists every entry wanted, and get looks one up. It reports
// whether anything changed. It reads only want, so the caller can hold the
// lock that guards it and save after letting go.
func (f *recordFile[T]) update(each func(yield func(string, T) bool), get func(string) (T, bool)) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	before := len(f.pending)
	var scratch []byte
	for p, v := range each {
		old, ok := f.held[p]
		if ok && old == v {
			continue
		}
		if ok {
			scratch = f.record(scratch[:0], recSet, p, old)
			f.live -= int64(len(scratch))
		}
		n := len(f.pending)
		f.pending = f.record(f.pending, recSet, p, v)
		f.size += int64(len(f.pending) - n)
		f.live += int64(len(f.pending) - n)
		f.held[p] = v
	}
	for p, old := range f.held {
		if _, ok := get(p); ok {
			continue
		}
		scratch = f.record(scratch[:0], recSet, p, old)
		f.live -= int64(len(scratch))
		n := len(f.pending)
		var zero T
		f.pending = f.record(f.pending, recDelete, p, zero)
		f.size += int64(len(f.pending) - n)
		delete(f.held, p)
	}
	return len(f.pending) != before
}

// garbage is the bytes of records that later ones replaced.
func (f *recordFile[T]) garbage() int64 { return f.size - f.live }

// save writes what is pending: appended, or the whole file afresh when the
// garbage would outweigh what it holds, when it was damaged, of something
// else, or not there, or when it is not as this left it (another sync wrote
// it meanwhile).
func (f *recordFile[T]) save() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 && !f.afresh {
		return nil
	}
	if !f.afresh && f.garbage() <= max(f.live, recMinGarbage) {
		switch err := f.append(); {
		case err == nil:
			return nil
		case !errors.Is(err, errRecordMoved):
			f.afresh = true
			return err
		}
	}
	return f.rewrite()
}

var errRecordMoved = errors.New("the file is not as it was left")

func (f *recordFile[T]) append() error {
	file, err := os.OpenFile(f.path, os.O_WRONLY|os.O_APPEND, 0)
	if os.IsNotExist(err) {
		return errRecordMoved
	}
	if err != nil {
		return err
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || info.Size() != f.written {
		return errRecordMoved
	}
	if _, err := file.Write(f.pending); err != nil {
		return err
	}
	f.written += int64(len(f.pending))
	f.pending = f.pending[:0]
	return nil
}

func (f *recordFile[T]) rewrite() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), filepath.Base(f.path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	buf := f.header()
	var total int64
	for p, v := range f.held {
		buf = f.record(buf, recSet, p, v)
		if len(buf) >= 1<<20 {
			if _, err := tmp.Write(buf); err != nil {
				tmp.Close()
				return err
			}
			total += int64(len(buf))
			buf = buf[:0]
		}
	}
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		return err
	}
	total += int64(len(buf))
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		return err
	}
	f.pending = f.pending[:0]
	f.size, f.written, f.afresh = f.live, total, false
	return nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// recReader reads the fields of a record, noting when one runs past its
// end.
type recReader struct {
	b   []byte
	bad bool
}

func (r *recReader) take(n int) []byte {
	if r.bad || n < 0 || n > len(r.b) {
		r.bad = true
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *recReader) byte() byte {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *recReader) uint32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *recReader) uvarint() uint64 {
	if r.bad {
		return 0
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.bad = true
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *recReader) varint() int64 {
	if r.bad {
		return 0
	}
	v, n := binary.Varint(r.b)
	if n <= 0 {
		r.bad = true
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *recReader) str() string {
	n := r.uvarint()
	if n > recMax {
		r.bad = true
		return ""
	}
	return string(r.take(int(n)))
}

// describeRecordErr says what is wrong with a record file, for a message.
func describeRecordErr(path string, err error) error {
	switch {
	case errors.Is(err, errRecordForeign):
		return fmt.Errorf("%s is not a knot sync file: remove it or choose another", path)
	case errors.Is(err, errRecordVersion):
		return fmt.Errorf("%s was written by another version of knot: remove it to start afresh", path)
	}
	return err
}
