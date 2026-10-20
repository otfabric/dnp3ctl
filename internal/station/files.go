package station

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// memEntry is one file of the outstation.
type memEntry struct {
	data    []byte
	created app.Timestamp
	perm    app.Permissions
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
}

func newFileServer(m *Model, log func(types.ServerEvent), now func() time.Time) (*fileServer, error) {
	fs := &fileServer{files: map[string]*memEntry{}, accept: m.Uploads.Accept, log: log, now: now}
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
		out = append(out, app.FileDescriptor{Name: child, Type: app.FileTypeSimple, Size: uint32(len(e.data)), Created: e.created, Permissions: e.perm})
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
			return &readFile{r: bytes.NewReader(e.data), size: uint32(len(e.data)), done: func(abort bool) { s.sent(name, len(e.data), abort) }}, app.FileSuccess
		}
		if s.isDir(name) {
			var b []byte
			for _, d := range s.list(name) {
				if nb, err := app.AppendFileDescriptor(b, d); err == nil {
					b = nb
				}
			}
			s.event(types.ServerEvent{Event: "file", Detail: "directory " + name})
			return &readFile{r: bytes.NewReader(b), size: uint32(len(b))}, app.FileSuccess
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
		w := &writeFile{fs: s, name: name, perm: req.Permissions}
		if e, ok := s.files[name]; ok && req.Mode == app.FileModeAppend {
			w.buf.Write(e.data)
		}
		if w.perm == 0 {
			w.perm = 0o644
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
	if _, ok := s.files[clean]; !ok {
		return app.FileNotFound
	}
	delete(s.files, clean)
	if s.dir != "" {
		_ = os.Remove(filepath.Join(s.dir, filepath.FromSlash(strings.TrimPrefix(clean, "/"))))
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
		return app.FileDescriptor{Name: name, Type: app.FileTypeSimple, Size: uint32(len(e.data)), Created: e.created, Permissions: e.perm}, app.FileSuccess
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

// store keeps a written file, on disk too when the model names an upload directory.
func (s *fileServer) store(name string, data []byte, perm app.Permissions) error {
	if s.dir != "" {
		p := filepath.Join(s.dir, filepath.FromSlash(strings.TrimPrefix(name, "/")))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[name] = &memEntry{data: data, created: app.TimestampOf(s.now()), perm: perm}
	s.event(types.ServerEvent{Event: "file", Detail: fmt.Sprintf("received %s (%d octets)", path.Clean(name), len(data))})
	return nil
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
	fs   *fileServer
	name string
	perm app.Permissions
	buf  bytes.Buffer
}

func (f *writeFile) Read([]byte) (int, error)    { return 0, io.EOF }
func (f *writeFile) Write(p []byte) (int, error) { return f.buf.Write(p) }
func (f *writeFile) Size() uint32                { return uint32(f.buf.Len()) }
func (f *writeFile) Close(abort bool) error {
	if abort {
		f.fs.event(types.ServerEvent{Event: "file", Detail: "write of " + f.name + " aborted"})
		return nil
	}
	return f.fs.store(f.name, bytes.Clone(f.buf.Bytes()), f.perm)
}
