//go:build darwin

package command_files

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// On macOS the directory is watched with FSEvents: one stream for the whole
// tree, however large, where kqueue would need an open file for every file.
// It is reached through goffi, as the knot command is built without cgo.

// newDirWatcher watches the directory with FSEvents, or fsnotify when FSEvents
// cannot be started.
func newDirWatcher(s *syncer) (dirWatcher, error) {
	w, err := newFSEventsWatcher(s)
	if err == nil {
		return w, nil
	}
	s.log.WithError(err).Warn("cannot use FSEvents: watching each folder instead")
	return newNotifyWatcher(s)
}

// FSEvents constants, from CoreServices' FSEvents.h.
const (
	fsevSinceNow = ^uint64(0) // kFSEventStreamEventIdSinceNow

	fsevCreateNoDefer   = 0x02 // kFSEventStreamCreateFlagNoDefer
	fsevCreateWatchRoot = 0x04 // kFSEventStreamCreateFlagWatchRoot
	fsevCreateFileEvent = 0x10 // kFSEventStreamCreateFlagFileEvents

	fsevMustScanSubDirs = 0x01 // kFSEventStreamEventFlagMustScanSubDirs
	fsevUserDropped     = 0x02
	fsevKernelDropped   = 0x04
	fsevIdsWrapped      = 0x08
	fsevRootChanged     = 0x20

	cfStringEncodingUTF8 = 0x08000100

	// fsevLatency is how long FSEvents gathers changes before reporting them.
	fsevLatency = 0.05
)

// fsevAPI is the part of CoreFoundation, CoreServices and libdispatch used.
type fsevAPI struct {
	stringCreate, arrayCreate, release                        unsafe.Pointer
	streamCreate, setQueue, start, stop, invalidate, release2 unsafe.Pointer
	queueCreate, queueRelease                                 unsafe.Pointer
	arrayCallBacks                                            unsafe.Pointer // &kCFTypeArrayCallBacks
	callback                                                  uintptr

	cifPtr2, cifPtr3, cifPtr4, cifVoid1, cifVoid2, cifBool1, cifCreate types.CallInterface
}

var (
	fsev     fsevAPI
	fsevErr  error
	fsevOnce sync.Once

	// streams routes FSEvents callbacks, which all go to one function, to
	// their watcher by the id given as the stream's context.
	streamsMu sync.Mutex
	streams   = map[uintptr]*fseventsWatcher{}
	streamSeq uintptr
)

func loadFSEvents() error {
	fsevOnce.Do(func() { fsevErr = fsev.load() })
	return fsevErr
}

func (a *fsevAPI) load() error {
	cf, err := ffi.LoadLibrary("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	if err != nil {
		return err
	}
	cs, err := ffi.LoadLibrary("/System/Library/Frameworks/CoreServices.framework/CoreServices")
	if err != nil {
		return err
	}
	sys, err := ffi.LoadLibrary("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return err
	}
	for _, sym := range []struct {
		lib  unsafe.Pointer
		name string
		to   *unsafe.Pointer
	}{
		{cf, "CFStringCreateWithCString", &a.stringCreate},
		{cf, "CFArrayCreate", &a.arrayCreate},
		{cf, "CFRelease", &a.release},
		{cf, "kCFTypeArrayCallBacks", &a.arrayCallBacks},
		{cs, "FSEventStreamCreate", &a.streamCreate},
		{cs, "FSEventStreamSetDispatchQueue", &a.setQueue},
		{cs, "FSEventStreamStart", &a.start},
		{cs, "FSEventStreamStop", &a.stop},
		{cs, "FSEventStreamInvalidate", &a.invalidate},
		{cs, "FSEventStreamRelease", &a.release2},
		{sys, "dispatch_queue_create", &a.queueCreate},
		{sys, "dispatch_release", &a.queueRelease},
	} {
		if *sym.to, err = ffi.GetSymbol(sym.lib, sym.name); err != nil {
			return fmt.Errorf("%s: %w", sym.name, err)
		}
	}

	ptr, u32, u64, i64, dbl, void := types.PointerTypeDescriptor, types.UInt32TypeDescriptor, types.UInt64TypeDescriptor,
		types.SInt64TypeDescriptor, types.DoubleTypeDescriptor, types.VoidTypeDescriptor
	for _, c := range []struct {
		cif  *types.CallInterface
		ret  *types.TypeDescriptor
		args []*types.TypeDescriptor
	}{
		{&a.cifPtr2, ptr, []*types.TypeDescriptor{ptr, ptr}},           // dispatch_queue_create
		{&a.cifPtr3, ptr, []*types.TypeDescriptor{ptr, ptr, u32}},      // CFStringCreateWithCString
		{&a.cifPtr4, ptr, []*types.TypeDescriptor{ptr, ptr, i64, ptr}}, // CFArrayCreate
		{&a.cifVoid1, void, []*types.TypeDescriptor{ptr}},              // CFRelease, stream stop, invalidate, release, dispatch_release
		{&a.cifVoid2, void, []*types.TypeDescriptor{ptr, ptr}},         // FSEventStreamSetDispatchQueue
		{&a.cifBool1, types.UInt8TypeDescriptor, []*types.TypeDescriptor{ptr}},
		{&a.cifCreate, ptr, []*types.TypeDescriptor{ptr, ptr, ptr, ptr, u64, dbl, u32}},
	} {
		if err := ffi.PrepareCallInterface(c.cif, types.DefaultCall, c.ret, c.args); err != nil {
			return err
		}
	}
	a.callback = ffi.NewCallback(fseventsCallback)
	return nil
}

// cstr is s with a terminating NUL, kept alive by the caller while in use.
func cstr(s string) []byte { return append([]byte(s), 0) }

func call(cif *types.CallInterface, fn unsafe.Pointer, ret unsafe.Pointer, args ...unsafe.Pointer) error {
	_, err := ffi.CallFunction(cif, fn, ret, args)
	return err
}

// fseventsWatcher watches a tree with one FSEvents stream.
type fseventsWatcher struct {
	s      *syncer
	id     uintptr
	stream unsafe.Pointer
	queue  unsafe.Pointer
	ch     chan watchEvent
	lost   atomic.Bool
	once   sync.Once
}

func newFSEventsWatcher(s *syncer) (*fseventsWatcher, error) {
	if err := loadFSEvents(); err != nil {
		return nil, err
	}
	streamsMu.Lock()
	streamSeq++
	w := &fseventsWatcher{s: s, id: streamSeq, ch: make(chan watchEvent, 8192)}
	streams[w.id] = w
	streamsMu.Unlock()
	if err := w.start(s.opt.dir); err != nil {
		streamsMu.Lock()
		delete(streams, w.id)
		streamsMu.Unlock()
		return nil, err
	}
	return w, nil
}

func (w *fseventsWatcher) start(dir string) error {
	var null unsafe.Pointer
	path := cstr(dir)
	pathPtr := unsafe.Pointer(&path[0])
	enc := uint32(cfStringEncodingUTF8)
	var cfPath unsafe.Pointer
	if err := call(&fsev.cifPtr3, fsev.stringCreate, unsafe.Pointer(&cfPath),
		unsafe.Pointer(&null), unsafe.Pointer(&pathPtr), unsafe.Pointer(&enc)); err != nil || cfPath == nil {
		return errors.Join(errors.New("CFStringCreateWithCString failed"), err)
	}
	defer w.cfRelease(cfPath)
	runtime.KeepAlive(path)

	values := [1]unsafe.Pointer{cfPath}
	valuesPtr := unsafe.Pointer(&values[0])
	count := int64(1)
	var paths unsafe.Pointer
	if err := call(&fsev.cifPtr4, fsev.arrayCreate, unsafe.Pointer(&paths),
		unsafe.Pointer(&null), unsafe.Pointer(&valuesPtr), unsafe.Pointer(&count), unsafe.Pointer(&fsev.arrayCallBacks)); err != nil || paths == nil {
		return errors.Join(errors.New("CFArrayCreate failed"), err)
	}
	defer w.cfRelease(paths)
	runtime.KeepAlive(values)

	// FSEventStreamContext: version, info, retain, release, copyDescription.
	// The stream copies it; info carries the watcher's id.
	ctx := [5]uintptr{0, w.id, 0, 0, 0}
	ctxPtr := unsafe.Pointer(&ctx[0])
	cb := fsev.callback
	since := fsevSinceNow
	latency := float64(fsevLatency)
	flags := uint32(fsevCreateNoDefer | fsevCreateWatchRoot | fsevCreateFileEvent)
	if err := call(&fsev.cifCreate, fsev.streamCreate, unsafe.Pointer(&w.stream),
		unsafe.Pointer(&null), unsafe.Pointer(&cb), unsafe.Pointer(&ctxPtr), unsafe.Pointer(&paths),
		unsafe.Pointer(&since), unsafe.Pointer(&latency), unsafe.Pointer(&flags)); err != nil || w.stream == nil {
		return errors.Join(errors.New("FSEventStreamCreate failed"), err)
	}
	runtime.KeepAlive(ctx)

	label := cstr("knot.files.sync")
	labelPtr := unsafe.Pointer(&label[0])
	if err := call(&fsev.cifPtr2, fsev.queueCreate, unsafe.Pointer(&w.queue),
		unsafe.Pointer(&labelPtr), unsafe.Pointer(&null)); err != nil || w.queue == nil {
		w.releaseStream()
		return errors.Join(errors.New("dispatch_queue_create failed"), err)
	}
	runtime.KeepAlive(label)
	if err := call(&fsev.cifVoid2, fsev.setQueue, nil, unsafe.Pointer(&w.stream), unsafe.Pointer(&w.queue)); err != nil {
		w.releaseStream()
		return err
	}
	var ok uint8
	if err := call(&fsev.cifBool1, fsev.start, unsafe.Pointer(&ok), unsafe.Pointer(&w.stream)); err != nil || ok == 0 {
		w.releaseStream()
		return errors.Join(errors.New("FSEventStreamStart failed"), err)
	}
	return nil
}

func (w *fseventsWatcher) cfRelease(p unsafe.Pointer) {
	call(&fsev.cifVoid1, fsev.release, nil, unsafe.Pointer(&p))
}

// releaseStream stops the stream, after which no more callbacks come, and
// frees it and its queue.
func (w *fseventsWatcher) releaseStream() {
	if w.stream != nil {
		call(&fsev.cifVoid1, fsev.stop, nil, unsafe.Pointer(&w.stream))
		call(&fsev.cifVoid1, fsev.invalidate, nil, unsafe.Pointer(&w.stream))
		call(&fsev.cifVoid1, fsev.release2, nil, unsafe.Pointer(&w.stream))
		w.stream = nil
	}
	if w.queue != nil {
		call(&fsev.cifVoid1, fsev.queueRelease, nil, unsafe.Pointer(&w.queue))
		w.queue = nil
	}
}

func (w *fseventsWatcher) add(string)                {} // the stream covers the whole tree
func (w *fseventsWatcher) remove(string)             {}
func (w *fseventsWatcher) events() <-chan watchEvent { return w.ch }
func (w *fseventsWatcher) takeLost() bool            { return w.lost.Swap(false) }

func (w *fseventsWatcher) close() {
	w.once.Do(func() {
		w.releaseStream()
		streamsMu.Lock()
		delete(streams, w.id)
		streamsMu.Unlock()
	})
}

// send passes an event on, never blocking FSEvents' thread: when the
// channel is full the events are lost, and everything is looked at.
func (w *fseventsWatcher) send(ev watchEvent) {
	select {
	case w.ch <- ev:
	default:
		w.lost.Store(true)
	}
}

// fseventsCallback receives every stream's events, on a dispatch queue's
// thread: FSEventStreamCallback(stream, info, numEvents, eventPaths,
// eventFlags, eventIds), the paths C strings.
func fseventsCallback(stream unsafe.Pointer, info uintptr, n uintptr, paths unsafe.Pointer, flags unsafe.Pointer, ids unsafe.Pointer) {
	streamsMu.Lock()
	w := streams[info]
	streamsMu.Unlock()
	if w == nil || n == 0 {
		return
	}
	pathList := unsafe.Slice((*unsafe.Pointer)(paths), n)
	flagList := unsafe.Slice((*uint32)(flags), n)
	for i := uintptr(0); i < n; i++ {
		f := flagList[i]
		switch {
		case f&(fsevUserDropped|fsevKernelDropped|fsevIdsWrapped|fsevRootChanged) != 0:
			w.send(watchEvent{all: true})
		default:
			w.send(watchEvent{path: goString(pathList[i]), below: f&fsevMustScanSubDirs != 0})
		}
	}
}

// goString copies a NUL-terminated C string.
func goString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}
