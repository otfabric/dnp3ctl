// SPDX-License-Identifier: MIT

package station

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The files a model names by a relative path stay inside the directory of the model file;
// an absolute path is read as it is.
func TestModelFilesStayInTheirDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside.bin")
	if err := os.WriteFile(outside, []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(sub, "model.yaml")
	load := func(path string) error {
		body := "points: [{type: binary-input, index: 0, value: on}]\nfiles: [{name: /f.bin, path: '" + path + "'}]\n"
		if err := os.WriteFile(model, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Load(model)
		return err
	}
	if err := load("../outside.bin"); err == nil || !strings.Contains(err.Error(), "outside.bin") {
		t.Errorf("a path out of the model directory was read: %v", err)
	}
	if err := load(outside); err != nil {
		t.Errorf("an absolute path: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(sub, "link.bin")); err == nil {
		if err := load("link.bin"); err == nil {
			t.Error("a symbolic link out of the model directory was followed")
		}
	}
	if err := os.WriteFile(filepath.Join(sub, "in.bin"), []byte{4}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := load("./x/../in.bin"); err != nil {
		t.Errorf("a path inside the model directory: %v", err)
	}

	// A model whose directory is missing reads nothing; a model that was not loaded from a
	// file reads relative paths below the working directory.
	m := &Model{dir: filepath.Join(dir, "missing")}
	if _, err := m.readFile("in.bin"); err == nil {
		t.Error("a file of a missing model directory was read")
	}
	t.Chdir(sub)
	if data, err := (&Model{}).readFile("in.bin"); err != nil || len(data) != 1 {
		t.Errorf("relative to the working directory: %v, %v", data, err)
	}
}

// An upload is written below the upload directory, never through a symbolic link out of it.
func TestUploadsStayInTheirDirectory(t *testing.T) {
	dir := t.TempDir()
	up := filepath.Join(dir, "up")
	away := filepath.Join(dir, "away")
	if err := os.Mkdir(away, 0o700); err != nil {
		t.Fatal(err)
	}
	fs := &fileServer{dir: up}
	if err := fs.storeOnDisk("/a/b/c.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := fs.storeOnDisk("/top.txt", []byte("y")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"a/b/c.txt": "x", "top.txt": "y"} {
		if data, err := os.ReadFile(filepath.Clean(filepath.Join(up, name))); err != nil || string(data) != want {
			t.Errorf("%s: %q, %v", name, data, err)
		}
	}
	if err := os.Symlink(away, filepath.Join(up, "link")); err == nil {
		if err := fs.storeOnDisk("/link/escaped.txt", []byte("z")); err == nil {
			t.Error("an upload was written through a symbolic link out of the upload directory")
		}
		if _, err := os.Stat(filepath.Join(away, "escaped.txt")); err == nil {
			t.Error("the upload landed outside the upload directory")
		}
	}
	// Removing goes through the root as well: not through the link, and not in a missing
	// upload directory.
	if err := os.WriteFile(filepath.Join(away, "keep.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fs.removeOnDisk("/link/keep.txt")
	if _, err := os.Stat(filepath.Join(away, "keep.txt")); err != nil {
		t.Errorf("a file was removed through a symbolic link out of the upload directory: %v", err)
	}
	fs.removeOnDisk("/top.txt")
	if _, err := os.Stat(filepath.Join(up, "top.txt")); err == nil {
		t.Error("an upload was not removed")
	}
	(&fileServer{dir: filepath.Join(dir, "missing")}).removeOnDisk("/x")

	// The upload directory, or a directory below it, cannot be made: a file is in the way.
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&fileServer{dir: blocked}).storeOnDisk("/x", nil); err == nil {
		t.Error("an upload directory that is a file was used")
	}
	if err := fs.storeOnDisk("/a/b/c.txt/below", nil); err == nil {
		t.Error("a directory was made over a file")
	}
}

// The random simulation draws from crypto/rand: fractions in [0, 1) that differ.
func TestRandomFraction(t *testing.T) {
	seen := map[float64]bool{}
	for range 64 {
		f := randomFraction()
		if f < 0 || f >= 1 {
			t.Fatalf("fraction %v", f)
		}
		seen[f] = true
	}
	if len(seen) < 60 {
		t.Errorf("%d different fractions of 64", len(seen))
	}
}
