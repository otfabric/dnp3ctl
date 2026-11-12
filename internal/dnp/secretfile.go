// SPDX-License-Identifier: MIT

package dnp

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// warnings is where a warning about a secret file goes: the stderr of the invocation.
var warnings = struct {
	mu   sync.Mutex
	w    io.Writer
	seen map[string]bool
}{w: os.Stderr}

// SetWarningOutput sends the warnings of this package to w, each once, until the returned
// function sends them to os.Stderr again. An invocation of the command tree sets it to its
// stderr.
func SetWarningOutput(w io.Writer) (restore func()) {
	warnings.mu.Lock()
	defer warnings.mu.Unlock()
	out := &warningOutput{w}
	warnings.w, warnings.seen = out, nil
	return func() {
		warnings.mu.Lock()
		defer warnings.mu.Unlock()
		if warnings.w == io.Writer(out) {
			warnings.w, warnings.seen = os.Stderr, nil
		}
	}
}

// warningOutput is a writer of SetWarningOutput, distinct for every call.
type warningOutput struct{ io.Writer }

// checkSecretFile warns when the file of a key or password (what) can be read by other
// users than its owner: on Unix, when its group or others have any permission. The secret is
// read all the same; the warning is printed once per file.
func checkSecretFile(path, what string) {
	mode, ok := loosePermissions(path)
	if !ok {
		return
	}
	warnings.mu.Lock()
	defer warnings.mu.Unlock()
	if warnings.seen[path] {
		return
	}
	if warnings.seen == nil {
		warnings.seen = map[string]bool{}
	}
	warnings.seen[path] = true
	_, _ = fmt.Fprintf(warnings.w, "warning: the %s file %s is accessible to other users (mode %04o): restrict it to its owner, for example with chmod 600\n", what, path, mode)
}
