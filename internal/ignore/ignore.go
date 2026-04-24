// Package ignore implements .driftignore file support: loading a set of
// glob patterns from a file and testing whether a resource's type.name
// identifier matches any of them.
//
// The file format is intentionally minimal and gitignore-adjacent:
//
//   - Lines beginning with '#' (after whitespace trimming) are comments.
//   - Blank lines are ignored.
//   - Every other line is a glob pattern matched against the resource's
//     primary label: "<type>.<name>" when the resource has a Name,
//     "<type>.<id>" when it does not (for example an EC2 instance with
//     no Name tag). This keeps patterns consistent with the labels the
//     renderer prints, so a user copying a label off screen into
//     .driftignore actually matches it.
//
// Globs follow path.Match semantics (asterisk matches any run of non-'/'
// characters, question mark matches one character, square brackets define
// character classes). Invalid patterns are rejected at parse time with
// the line number of the offending entry so users do not discover
// malformed ignores only when a scan fails to suppress the expected
// resources.
package ignore

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DefaultFileName is the conventional on-disk name of a driftignore file.
// It matches gitignore's dotfile convention and is the name Discover
// searches for.
const DefaultFileName = ".driftignore"

// Patterns holds the glob patterns parsed from a driftignore file plus
// a human-readable source identifier used in error messages and surfaced
// via Source().
//
// The zero value is valid and matches nothing; a nil *Patterns is also
// valid and handled by every method.
type Patterns struct {
	patterns []string
	source   string
}

// Parse reads driftignore content from r and returns a Patterns pointer
// containing every non-blank non-comment line. source is propagated into
// parse errors and stored on the returned value for later display.
//
// Each pattern is validated eagerly via path.Match so malformed globs
// (unbalanced '[' for instance) fail immediately at parse time with a
// line-number-qualified error rather than silently never matching at
// scan time.
func Parse(r io.Reader, source string) (*Patterns, error) {
	var patterns []string
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := path.Match(line, ""); err != nil {
			return nil, fmt.Errorf("%s: line %d: invalid pattern %q: %w", source, lineNum, line, err)
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: read error: %w", source, err)
	}
	return &Patterns{patterns: patterns, source: source}, nil
}

// LoadFile opens path, parses it as a driftignore file, and returns the
// resulting Patterns. A missing file is surfaced as a descriptive error;
// callers that want "missing file is fine" semantics should use Discover
// instead.
func LoadFile(filePath string) (*Patterns, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer f.Close()
	return Parse(f, filePath)
}

// Discover locates a driftignore file using gitignore-style lookup:
//
//  1. startDir/.driftignore (current working directory) wins if present.
//  2. Otherwise the filesystem is walked upward from startDir looking
//     for a .git marker; if one is found and it is a different directory,
//     <gitroot>/.driftignore is used if present.
//  3. If neither location contains a file, an empty Patterns with
//     Source() == "" is returned without an error. "No ignore file" is
//     a supported steady state, not an exceptional condition.
//
// Discover only reads regular files; a .driftignore directory is ignored.
func Discover(startDir string) (*Patterns, error) {
	if p, ok, err := loadIfRegular(filepath.Join(startDir, DefaultFileName)); err != nil {
		return nil, err
	} else if ok {
		return p, nil
	}

	root, ok := findGitRoot(startDir)
	if ok && root != startDir {
		if p, ok, err := loadIfRegular(filepath.Join(root, DefaultFileName)); err != nil {
			return nil, err
		} else if ok {
			return p, nil
		}
	}

	return &Patterns{}, nil
}

// loadIfRegular returns (patterns, true, nil) when filePath refers to a
// readable regular file, (nil, false, nil) when the file does not exist
// or is a directory, and (nil, false, err) on any other stat/open error.
// Treating "does not exist" as a non-error is what makes Discover's
// fallback chain work without sentinel checking at the call site.
func loadIfRegular(filePath string) (*Patterns, bool, error) {
	st, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat %s: %w", filePath, err)
	}
	if st.IsDir() {
		return nil, false, nil
	}
	p, err := LoadFile(filePath)
	if err != nil {
		return nil, false, err
	}
	return p, true, nil
}

// findGitRoot walks upward from startDir looking for a .git entry (file
// or directory; git worktrees use a file). It returns the directory that
// contains .git along with ok=true, or ("", false) when the traversal
// reaches the filesystem root without finding one.
func findGitRoot(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Match reports whether identity (typically "type.name") matches any of
// p's patterns. Matching is case-sensitive and follows path.Match glob
// semantics. A nil receiver or an empty Patterns always returns false so
// callers never need to guard against "no patterns loaded".
func (p *Patterns) Match(identity string) bool {
	if p == nil {
		return false
	}
	for _, pat := range p.patterns {
		if ok, _ := path.Match(pat, identity); ok {
			return true
		}
	}
	return false
}

// IsEmpty reports whether p contains no patterns. A nil receiver is
// considered empty.
func (p *Patterns) IsEmpty() bool {
	return p == nil || len(p.patterns) == 0
}

// Len returns the number of parsed patterns. A nil receiver returns 0.
func (p *Patterns) Len() int {
	if p == nil {
		return 0
	}
	return len(p.patterns)
}

// Source returns the origin of the patterns as supplied to Parse or
// LoadFile. Discover sets it to the path actually read; if no file was
// found the value is the empty string.
func (p *Patterns) Source() string {
	if p == nil {
		return ""
	}
	return p.source
}
