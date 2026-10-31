// SPDX-License-Identifier: MIT

package dnp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-dnp3/app"
)

// ParseObject parses an object as the flags take it: g30v5, 30v5, 30.5 or 30:5 for a group
// and variation, g30, 30 or g30v0 for any variation of group 30. The object must be one
// go-dnp3 knows.
func ParseObject(s string) (app.GroupVariation, error) {
	in := strings.ToLower(strings.TrimSpace(s))
	in = strings.TrimPrefix(in, "g")
	group, variation := in, "0"
	for _, sep := range []string{"v", ".", ":"} {
		if g, v, ok := strings.Cut(in, sep); ok {
			group, variation = g, v
			break
		}
	}
	g, gerr := strconv.ParseUint(group, 10, 8)
	v, verr := strconv.ParseUint(variation, 10, 8)
	if in == "" || gerr != nil || verr != nil {
		return 0, fmt.Errorf("invalid object %q: use g30v5 (or 30.5) for a variation, g30 for any variation", s)
	}
	gv := app.GV(uint8(g), uint8(v))
	if !gv.Known() {
		return 0, fmt.Errorf("unknown object %q: see dnp3ctl client objects", s)
	}
	return gv, nil
}

// ParseObjects parses a comma-separated list of objects.
func ParseObjects(list string) ([]app.GroupVariation, error) {
	var out []app.GroupVariation
	for _, s := range strings.Split(list, ",") {
		if strings.TrimSpace(s) == "" {
			continue
		}
		gv, err := ParseObject(s)
		if err != nil {
			return nil, err
		}
		out = append(out, gv)
	}
	return out, nil
}

// ObjectNames returns the names of gvs as the results show them ("g30v5").
func ObjectNames(gvs []app.GroupVariation) []string {
	out := make([]string, 0, len(gvs))
	for _, gv := range gvs {
		out = append(out, gv.String())
	}
	return out
}

// Selection names points of an object: all of them, a range, a list or a count.
type Selection struct {
	All         bool
	Start, Stop uint32
	List        []uint32
	Count       uint32
}

// ParseSelection parses --range ("" for all points, "0-9" for a range, "1,4,7" for a list,
// "5" for one point) together with --count (at most this many objects).
func ParseSelection(rng string, count uint32) (Selection, error) {
	rng = strings.TrimSpace(rng)
	switch {
	case rng != "" && count != 0:
		return Selection{}, fmt.Errorf("--range and --count are mutually exclusive")
	case count != 0:
		return Selection{Count: count}, nil
	case rng == "":
		return Selection{All: true}, nil
	case strings.Contains(rng, ","):
		var list []uint32
		for _, part := range strings.Split(rng, ",") {
			n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
			if err != nil {
				return Selection{}, fmt.Errorf("invalid index %q in --range %q: use a range (0-9) or a list of indexes (1,4,7)", part, rng)
			}
			list = append(list, uint32(n))
		}
		return Selection{List: list}, nil
	}
	lo, hi, isRange := strings.Cut(rng, "-")
	a, aerr := strconv.ParseUint(strings.TrimSpace(lo), 10, 32)
	b := a
	var berr error
	if isRange {
		b, berr = strconv.ParseUint(strings.TrimSpace(hi), 10, 32)
	}
	if aerr != nil || berr != nil {
		return Selection{}, fmt.Errorf("invalid --range %q: use a range (0-9), a list (1,4,7) or one index", rng)
	}
	if b < a {
		return Selection{}, fmt.Errorf("invalid --range %q: the end is below the start", rng)
	}
	return Selection{Start: uint32(a), Stop: uint32(b)}, nil
}

// String renders the selection for results ("0-9", "1,4,7", "count 5"; "" for all).
func (s Selection) String() string {
	switch {
	case s.All:
		return ""
	case s.Count != 0:
		return fmt.Sprintf("count %d", s.Count)
	case s.List != nil:
		parts := make([]string, 0, len(s.List))
		for _, n := range s.List {
			parts = append(parts, strconv.FormatUint(uint64(n), 10))
		}
		return strings.Join(parts, ",")
	case s.Start == s.Stop:
		return strconv.FormatUint(uint64(s.Start), 10)
	}
	return fmt.Sprintf("%d-%d", s.Start, s.Stop)
}

// Header returns the object header that names the selected points of gv.
func (s Selection) Header(gv app.GroupVariation) app.Header {
	switch {
	case s.All:
		return app.AllObjects(gv)
	case s.Count != 0:
		return app.CountOf(gv, s.Count)
	case s.List != nil:
		return app.IndexList(gv, s.List...)
	}
	return app.IndexRange(gv, s.Start, s.Stop)
}

// Headers returns one header per object.
func (s Selection) Headers(gvs []app.GroupVariation) []app.Header {
	out := make([]app.Header, 0, len(gvs))
	for _, gv := range gvs {
		out = append(out, s.Header(gv))
	}
	return out
}

// ParseClasses parses a comma-separated list of classes. min is the lowest class allowed (0
// or 1); an empty list is def.
func ParseClasses(list string, minClass int, def []int) ([]int, error) {
	if strings.TrimSpace(list) == "" {
		return def, nil
	}
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(list, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < minClass || n > 3 {
			return nil, fmt.Errorf("invalid class %q: use %d-3", strings.TrimSpace(part), minClass)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, nil
}

// IndexFilter is a set of point indexes given as a list and ranges ("0,5-9").
type IndexFilter struct {
	ranges [][2]uint32
}

// ParseIndexFilter parses --index. An empty string accepts every index.
func ParseIndexFilter(s string) (IndexFilter, error) {
	var f IndexFilter
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, aerr := strconv.ParseUint(strings.TrimSpace(lo), 10, 32)
		b := a
		var berr error
		if isRange {
			b, berr = strconv.ParseUint(strings.TrimSpace(hi), 10, 32)
		}
		if aerr != nil || berr != nil || b < a {
			return f, fmt.Errorf("invalid index %q: use indexes and ranges, e.g. 0,5-9", part)
		}
		f.ranges = append(f.ranges, [2]uint32{uint32(a), uint32(b)})
	}
	return f, nil
}

// Match reports whether index is in the filter.
func (f IndexFilter) Match(index uint32) bool {
	if len(f.ranges) == 0 {
		return true
	}
	for _, r := range f.ranges {
		if index >= r[0] && index <= r[1] {
			return true
		}
	}
	return false
}

// ObjectFilter is a set of groups and group/variations ("g30,g1v2").
type ObjectFilter struct {
	set map[app.GroupVariation]bool
}

// ParseObjectFilter parses --object of poll and monitor. An empty string accepts every
// object.
func ParseObjectFilter(s string) (ObjectFilter, error) {
	gvs, err := ParseObjects(s)
	if err != nil {
		return ObjectFilter{}, err
	}
	f := ObjectFilter{}
	if len(gvs) > 0 {
		f.set = map[app.GroupVariation]bool{}
		for _, gv := range gvs {
			f.set[gv] = true
		}
	}
	return f, nil
}

// Match reports whether gv is in the filter: listed itself, or its group listed as "any
// variation".
func (f ObjectFilter) Match(gv app.GroupVariation) bool {
	return f.set == nil || f.set[gv] || f.set[app.GV(gv.Group(), 0)]
}

// ParseTime parses --time: "now" (or empty) for the current time, otherwise RFC 3339.
func ParseTime(s string, now func() time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "now") {
		return now(), nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --time %q: use now or an RFC 3339 time such as 2026-01-31T12:00:00Z", s)
	}
	return t, nil
}

// ParsePermissions parses --permissions: octal ("644", "0644") or the rwx form
// ("rw-r--r--"). An empty string is rw-r--r--.
func ParsePermissions(s string) (app.Permissions, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0o644, nil
	}
	if n, err := strconv.ParseUint(s, 8, 16); err == nil && n <= 0o777 {
		return app.Permissions(n), nil
	}
	if len(s) == 9 {
		var p app.Permissions
		for i, c := range s {
			want := "rwx"[i%3]
			switch {
			case byte(c) == want:
				p |= 1 << (8 - i)
			case c != '-':
				return 0, fmt.Errorf("invalid --permissions %q: use octal (644) or the rwx form (rw-r--r--)", s)
			}
		}
		return p, nil
	}
	return 0, fmt.Errorf("invalid --permissions %q: use octal (644) or the rwx form (rw-r--r--)", s)
}

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[uint8]V) []uint8 {
	keys := make([]uint8, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
