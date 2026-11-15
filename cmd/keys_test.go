// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A key that is not hex is refused with invalid_key, and the message and its causes, which
// --debug prints, name the position of the first character that is not a hex digit but no
// character of the key: not in the form of encoding/hex (U+005A 'Z'), nor as the character
// itself or its code.
func TestKeyThatIsNotHexIsNotPrinted(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name, flag, env string
		content         []byte
		position        string
	}{
		{"binary key file", "--sa-key-file", "", []byte{0x5a, 0x9c, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}, "character 1 "},
		{"update key file with a mistyped digit", "--sa-update-key-file", "", []byte("00112233445566778899aabbccddeeqf\n"), "character 31 "},
		{"key in the environment", "", "DNP3CTL_SA_KEY", []byte("0011223344556677889#aabbccddeeff"), "character 20 "},
		{"update key in the environment", "", "DNP3CTL_SA_UPDATE_KEY", []byte("Zq00112233445566778899aabbccddee"), "character 1 "},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := []string{"--debug", "client", "poll", "--ip", "127.0.0.1", "--timeout", "1000"}
			if c.flag != "" {
				file := filepath.Join(dir, strings.ReplaceAll(c.name, " ", "-"))
				if err := os.WriteFile(file, c.content, 0o600); err != nil {
					t.Fatal(err)
				}
				args = append(args, c.flag, file)
			} else {
				t.Setenv(c.env, string(c.content))
			}
			for _, format := range []string{"text", "json"} {
				r := run(t, append(args, "--format", format)...).want(t, 3)
				out := r.stdout + r.stderr
				if !strings.Contains(out, "invalid_key") && !strings.Contains(out, "must be hex") {
					t.Errorf("%s: the error is not about the key:\n%s", format, out)
				}
				if format == "text" && !strings.Contains(r.stderr, c.position) {
					t.Errorf("%s: stderr does not name the %q:\n%s", format, c.position, r.stderr)
				}
				for _, leak := range []string{"U+00", "invalid byte"} {
					if strings.Contains(out, leak) {
						t.Errorf("%s: the output holds %q:\n%s", format, leak, out)
					}
				}
				for _, b := range c.content {
					if strings.ContainsRune("0123456789abcdefABCDEF\n", rune(b)) {
						continue
					}
					for _, form := range []string{fmt.Sprintf("'%c'", b), fmt.Sprintf("%q", rune(b)), fmt.Sprintf("U+%04X", b), fmt.Sprintf("0x%02x", b), fmt.Sprintf(`\x%02x`, b)} {
						if strings.Contains(out, form) {
							t.Errorf("%s: the output holds the octet 0x%02x of the key as %s:\n%s", format, b, form, out)
						}
					}
				}
			}
		})
	}
}
