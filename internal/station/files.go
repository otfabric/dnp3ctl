// SPDX-License-Identifier: MIT

package station

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/server"

	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// maxFileSize is the largest file a master may write: the largest size of the four
// octets of g70 (a variable, so that tests can lower it).
var maxFileSize int64 = math.MaxUint32

// size32 returns n as the four-octet size of a g70 file: n clamped to 0..4 GiB - 1.
func size32(n int) uint32 { return uint32(min(uint64(max(n, 0)), math.MaxUint32)) }

// memEntry is one file of the outstation.
type memEntry struct {
	data    []byte
	created app.Timestamp
	perm    app.Permissions
	// uploaded is set for a file a master wrote: it counts toward the upload limits.
	uploaded bool
}

// fileServer serves the files of a model from memory, takes uploads when the model accepts
// them and lists directories, which are implied by the names of the files.
type fileServer struct {
	mu     sync.Mutex
	files  map[string]*memEntry
	accept bool
	dir    string
	log    func(types.ServerEvent)
	now    func() time.Time

	// maxFiles and maxBytes are the upload limits of the model. What counts toward them, under
	// mu: the files masters wrote (uploads) and their octets (held), the transfers in progress
	// to a name that holds no upload yet (adding), and the octets masters have sent in the
	// transfers in progress (sending).
	maxFiles int
	maxBytes int64
	uploads  int
	held     int64
	adding   int
	sending  int64
}

func newFileServer(m *Model, log func(types.ServerEvent), now func() time.Time) (*fileServer, error) {
	fs := &fileServer{files: map[string]*memEntry{}, accept: m.Uploads.Accept, log: log, now: now}
	fs.maxFiles, fs.maxBytes = m.Uploads.limits()
	if m.Uploads.Dir != "" {
		fs.dir = m.resolve(m.Uploads.Dir)
	}
	created := app.TimestampOf(now())
	for _, f := range m.Files {
		name, err := cleanName(f.Name)
		if err != nil {
			return nil, err
		}
		data, err := m.fileContent(f)
		if err != nil {
			return nil, fmt.Errorf("file %s: %w", name, err)
		}
		perm, err := dnp.ParsePermissions(f.Permissions)
		if err != nil {
			return nil, fmt.Errorf("file %s: %w", name, err)
		}
		fs.files[name] = &memEntry{data: data, created: created, perm: perm}
	}
	return fs, nil
}

// isDir reports whether name is a directory: the root, or a prefix of a file. The caller
// holds mu.
func (s *fileServer) isDir(name string) bool {
	if name == "/" {
		return true
	}
	for n := range s.files {
		if strings.HasPrefix(n, name+"/") {
			return true
		}
	}
	return false
}

// list returns the descriptors of the entries of directory name, files and subdirectories.
// The caller holds mu.
func (s *fileServer) list(name string) []app.FileDescriptor {
	prefix := strings.TrimSuffix(name, "/") + "/"
	seen := map[string]bool{}
	var out []app.FileDescriptor
	for n, e := range s.files {
		rest, ok := strings.CutPrefix(n, prefix)
		if !ok {
			continue
		}
		child, _, deeper := strings.Cut(rest, "/")
		if seen[child] {
			continue
		}
		seen[child] = true
		if deeper {
			out = append(out, app.FileDescriptor{Name: child, Type: app.FileTypeDirectory, Created: e.created, Permissions: 0o755})
			continue
		}
		out = append(out, app.FileDescriptor{Name: child, Type: app.FileTypeSimple, Size: size32(len(e.data)), Created: e.created, Permissions: e.perm})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Open implements server.FileServer.
func (s *fileServer) Open(_ context.Context, req server.FileRequest) (server.File, app.FileStatus) {
	name, err := cleanName(req.Name)
	if err != nil {
		return nil, app.FileNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch req.Mode {
	case app.FileModeRead:
		if e, ok := s.files[name]; ok {
			return &readFile{r: bytes.NewReader(e.data), size: size32(len(e.data)), done: func(abort bool) { s.sent(name, len(e.data), abort) }}, app.FileSuccess
		}
		if s.isDir(name) {
			var b []byte
			for _, d := range s.list(name) {
				if nb, err := app.AppendFileDescriptor(b, d); err == nil {
					b = nb
				}
			}
			s.event(types.ServerEvent{Event: "file", Detail: "directory " + name})
			return &readFile{r: bytes.NewReader(b), size: size32(len(b))}, app.FileSuccess
		}
		return nil, app.FileNotFound
	case app.FileModeWrite, app.FileModeAppend:
		if !s.accept {
			s.event(types.ServerEvent{Event: "file", Detail: "write of " + name + " refused: uploads are not accepted"})
			return nil, app.FilePermissionDenied
		}
		if s.isDir(name) {
			return nil, app.FilePermissionDenied
		}
		// A write to a name that holds no upload yet adds a file once it completes: it counts
		// toward uploads.max_files from the open.
		prev := s.files[name]
		adds := prev == nil || !prev.uploaded
		if adds && s.uploads+s.adding >= s.maxFiles {
			s.event(types.ServerEvent{Event: "file", Detail: fmt.Sprintf("write of %s refused: the outstation holds %d files masters wrote, the most uploads.max_files allows", name, s.maxFiles)})
			return nil, app.FilePermissionDenied
		}
		// A new file gets the time of creation the master asks for, or now; one appended
		// to keeps its own.
		w := &writeFile{fs: s, name: name, perm: req.Permissions, created: req.Created, adds: adds}
		if prev != nil && req.Mode == app.FileModeAppend {
			w.buf.Write(prev.data)
			w.created = prev.created
		}
		if w.perm == 0 {
			w.perm = 0o644
		}
		if adds {
			s.adding++
		}
		return w, app.FileSuccess
	default:
		return nil, app.FileInvalidMode
	}
}

// Delete implements server.FileServer.
func (s *fileServer) Delete(_ context.Context, name string, _ uint32) app.FileStatus {
	clean, err := cleanName(name)
	if err != nil {
		return app.FileNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.accept {
		s.event(types.ServerEvent{Event: "file", Detail: "delete of " + clean + " refused: uploads are not accepted"})
		return app.FilePermissionDenied
	}
	e, ok := s.files[clean]
	if !ok {
		return app.FileNotFound
	}
	if e.uploaded {
		s.uploads--
		s.held -= int64(len(e.data))
	}
	delete(s.files, clean)
	if s.dir != "" {
		s.removeOnDisk(clean)
	}
	s.event(types.ServerEvent{Event: "file", Detail: "deleted " + clean})
	return app.FileSuccess
}

// Info implements server.FileServer.
func (s *fileServer) Info(_ context.Context, name string) (app.FileDescriptor, app.FileStatus) {
	clean, err := cleanName(name)
	if err != nil {
		return app.FileDescriptor{}, app.FileNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.files[clean]; ok {
		return app.FileDescriptor{Name: name, Type: app.FileTypeSimple, Size: size32(len(e.data)), Created: e.created, Permissions: e.perm}, app.FileSuccess
	}
	if s.isDir(clean) {
		return app.FileDescriptor{Name: name, Type: app.FileTypeDirectory, Created: app.TimestampOf(s.now()), Permissions: 0o755}, app.FileSuccess
	}
	return app.FileDescriptor{}, app.FileNotFound
}

func (s *fileServer) event(e types.ServerEvent) {
	if s.log != nil {
		s.log(e)
	}
}

func (s *fileServer) sent(name string, n int, abort bool) {
	if abort {
		s.event(types.ServerEvent{Event: "file", Detail: fmt.Sprintf("read of %s aborted", name)})
		return
	}
	s.event(types.ServerEvent{Event: "file", Detail: fmt.Sprintf("sent %s (%d octets)", name, n)})
}

// storeOnDisk writes the file name (a clean absolute name, see cleanName) below the upload
// directory, through an [os.Root] of that directory: a name the master sends cannot reach
// outside it, through ".." or a symbolic link.
func (s *fileServer) storeOnDisk(name string, data []byte) error {
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	rel := filepath.FromSlash(strings.TrimPrefix(name, "/"))
	if dir := filepath.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return root.WriteFile(rel, data, 0o600)
}

// removeOnDisk removes the file name (a clean absolute name) below the upload directory,
// through an [os.Root] of that directory like storeOnDisk. A file that is not there is no
// error: the file may have been kept in memory only.
func (s *fileServer) removeOnDisk(name string) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return
	}
	defer func() { _ = root.Close() }()
	_ = root.Remove(filepath.FromSlash(strings.TrimPrefix(name, "/")))
}

// store keeps a written file, on disk too when the model names an upload directory. The
// transfer w, when not nil, ends in the same step, so that its octets count once: in the
// transfer or in the file.
func (s *fileServer) store(name string, data []byte, perm app.Permissions, created app.Timestamp, w *writeFile) error {
	if s.dir != "" {
		if err := s.storeOnDisk(name, data); err != nil {
			s.release(w)
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked(w)
	if created == 0 {
		created = app.TimestampOf(s.now())
	}
	if prev := s.files[name]; prev != nil && prev.uploaded {
		s.uploads--
		s.held -= int64(len(prev.data))
	}
	s.files[name] = &memEntry{data: data, created: created, perm: perm, uploaded: true}
	s.uploads++
	s.held += int64(len(data))
	s.event(types.ServerEvent{Event: "file", Detail: fmt.Sprintf("received %s (%d octets)", path.Clean(name), len(data))})
	return nil
}

// reserve counts n more octets of the transfer w toward uploads.max_bytes, or refuses them
// when the files masters wrote and the transfers in progress would then hold more.
func (s *fileServer) reserve(w *writeFile, n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held+s.sending+n > s.maxBytes {
		s.event(types.ServerEvent{Event: "file", Detail: fmt.Sprintf("write of %s refused: the files masters wrote would hold more than %d octets, the most uploads.max_bytes allows", w.name, s.maxBytes)})
		return fmt.Errorf("%s: the files masters wrote would hold more than %d octets, the most the outstation keeps", w.name, s.maxBytes)
	}
	s.sending += n
	w.sent += n
	return nil
}

// release ends the transfer w, which no longer counts toward the upload limits.
func (s *fileServer) release(w *writeFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked(w)
}

// releaseLocked is release with mu held; nil and a transfer that ended already do nothing.
func (s *fileServer) releaseLocked(w *writeFile) {
	if w == nil || w.done {
		return
	}
	w.done = true
	s.sending -= w.sent
	if w.adds {
		s.adding--
	}
}

// readFile is a file or directory listing opened for reading.
type readFile struct {
	r    *bytes.Reader
	size uint32
	done func(abort bool)
}

func (f *readFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *readFile) Write([]byte) (int, error)  { return 0, io.ErrClosedPipe }
func (f *readFile) Size() uint32               { return f.size }
func (f *readFile) Close(abort bool) error {
	if f.done != nil {
		f.done(abort)
	}
	return nil
}

// writeFile collects a file a master writes and stores it when the transfer completes.
type writeFile struct {
	fs      *fileServer
	name    string
	perm    app.Permissions
	created app.Timestamp
	buf     bytes.Buffer
	// adds is set when the name holds no upload yet, sent counts the octets the master sent,
	// and done is set once the transfer ended: the upload limits of fileServer, under its mu.
	adds bool
	sent int64
	done bool
}

func (f *writeFile) Read([]byte) (int, error) { return 0, io.EOF }
func (f *writeFile) Size() uint32             { return size32(f.buf.Len()) }

// Write takes a block of the file. A file of g70 has a size of four octets, so the
// file may not grow past 4 GiB - 1: a master that sends more gets an error, which the
// outstation answers with FILE_FATAL, instead of the size wrapping. A block that would
// take the files masters wrote past uploads.max_bytes gets an error as well.
func (f *writeFile) Write(p []byte) (int, error) {
	if int64(f.buf.Len())+int64(len(p)) > maxFileSize {
		return 0, fmt.Errorf("%s would exceed %d octets, the largest size of a DNP3 file", f.name, maxFileSize)
	}
	if err := f.fs.reserve(f, int64(len(p))); err != nil {
		return 0, err
	}
	return f.buf.Write(p)
}

// Close ends the transfer, and keeps the file unless it is aborted. The file keeps the
// octets of the buffer, which the transfer does not use again.
func (f *writeFile) Close(abort bool) error {
	if abort {
		f.fs.release(f)
		f.fs.event(types.ServerEvent{Event: "file", Detail: "write of " + f.name + " aborted"})
		return nil
	}
	return f.fs.store(f.name, f.buf.Bytes(), f.perm, f.created, f)
}
