// Package buffer is an in-house piece table with immutable snapshots.
package buffer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// Offset is a byte offset in a document.
type Offset int

// Origin classifies an edit so undo can group typing.
type Origin int

const (
	// OriginCommand is one undo step (paste, cut, an explicit edit).
	OriginCommand Origin = iota
	// OriginTyping merges with the previous typing edit until a pause,
	// a cursor jump, or another command.
	OriginTyping
	// OriginExternal is an edit that did not come from the user (reload).
	OriginExternal
)

// Edit replaces [Start, End) with Text. A batch is sorted and non-overlapping.
type Edit struct {
	Start, End Offset
	Text       string
}

// Document is one open file.
type Document interface {
	ID() uint64
	Path() string
	Version() int32
	Snapshot() Snapshot
	Apply(edits []Edit, origin Origin) (Snapshot, error)
	Undo() bool
	Redo() bool
	Dirty() bool
	Save(ctx context.Context) error
	ReadOnly() bool
	Notice() string
	SetPath(path string)
	BreakGroup()
}

// Snapshot is an immutable view of a document. It is safe to share.
type Snapshot interface {
	Version() int32
	Len() int
	LineCount() int
	Line(n int) []byte
	LineStart(n int) Offset
	OffsetToLineCol(o Offset) (line, col int)
	Slice(start, end Offset) []byte
	Bytes() []byte
}

var (
	ErrReadOnly = errors.New("document is read-only")
	ErrOverlap  = errors.New("edits overlap or are out of order")
	ErrRange    = errors.New("edit is outside the document")
	ErrNoPath   = errors.New("document has no path")
	errClosed   = errors.New("document is closed")
)

var nextDocID atomic.Uint64

// Doc is the mutable piece table.
type Doc struct {
	id       uint64
	path     string
	version  int32
	root     *node
	orig     []byte
	add      []byte
	crlf     bool
	dirty    bool
	readOnly bool
	notice   string
	undo     []hist
	redo     []hist
	lastOrig Origin
	lastType time.Time
	lastAt   Offset
	grouped  bool
	closed   bool
}

type hist struct {
	root    *node
	addLen  int
	version int32
	dirty   bool
}

// New returns an empty editable document.
func New(path string) *Doc {
	return &Doc{id: nextDocID.Add(1), path: path, version: 1, lastOrig: OriginExternal}
}

// Open reads path. Invalid UTF-8 and files that contain a NUL byte open
// read-only. CRLF files are stored as LF and written back as CRLF.
func Open(path string) (*Doc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromBytes(path, b), nil
}

// FromBytes builds a document from b. The slice is copied.
func FromBytes(path string, b []byte) *Doc {
	d := New(path)
	raw := append([]byte(nil), b...)
	if bytesLookBinary(raw) {
		d.readOnly = true
		d.notice = "This file is binary or not valid UTF-8. It is read-only."
		d.setOrig(raw)
		return d
	}
	if hasCRLF(raw) {
		d.crlf = true
		raw = stripCRLF(raw)
	}
	d.setOrig(raw)
	return d
}

// setOrig stores raw in fixed-size pieces so line lookup stays logarithmic
// on a large file.
func (d *Doc) setOrig(raw []byte) {
	d.orig = raw
	const chunk = 4096
	var root *node
	for i := 0; i < len(raw); i += chunk {
		end := min(i+chunk, len(raw))
		root = merge(root, leaf(0, i, end-i, countNL(raw[i:end])))
	}
	d.root = root
}

func bytesLookBinary(b []byte) bool {
	if bytes.Contains(b, []byte{0}) {
		return true
	}
	return !utf8.Valid(b)
}

func hasCRLF(b []byte) bool {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '\r' && b[i+1] == '\n' {
			return true
		}
	}
	return false
}

func stripCRLF(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := range b {
		if b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n' {
			continue
		}
		out = append(out, b[i])
	}
	return out
}

func (d *Doc) ID() uint64       { return d.id }
func (d *Doc) Path() string     { return d.path }
func (d *Doc) Version() int32   { return d.version }
func (d *Doc) Dirty() bool      { return d.dirty }
func (d *Doc) ReadOnly() bool   { return d.readOnly }
func (d *Doc) Notice() string   { return d.notice }
func (d *Doc) SetPath(p string) { d.path = p }
func (d *Doc) CRLF() bool       { return d.crlf }

// BreakGroup ends the current typing undo group. Call it on a cursor jump.
func (d *Doc) BreakGroup() { d.grouped = false }

func (d *Doc) Snapshot() Snapshot {
	return d.snap()
}

func (d *Doc) snap() *snap {
	return &snap{
		version: d.version,
		root:    d.root,
		orig:    d.orig,
		add:     d.add,
	}
}

// Apply applies edits as one transaction. edits must be sorted and non-overlapping.
func (d *Doc) Apply(edits []Edit, origin Origin) (Snapshot, error) {
	if d.closed {
		return nil, errClosed
	}
	if d.readOnly && origin != OriginExternal {
		return nil, ErrReadOnly
	}
	if err := d.validate(edits); err != nil {
		return nil, err
	}
	if len(edits) == 0 {
		return d.snap(), nil
	}
	now := time.Now()
	merge := origin == OriginTyping && d.grouped && d.lastOrig == OriginTyping &&
		now.Sub(d.lastType) <= time.Second && typingAdjacent(edits, d.lastAt)
	if !merge {
		d.pushHist()
		d.redo = nil
	}
	for _, e := range slices.Backward(edits) {

		d.root = d.remove(d.root, int(e.Start), int(e.End))
		d.root = d.insert(d.root, int(e.Start), e.Text)
	}
	d.version++
	d.dirty = true
	d.lastOrig = origin
	if origin == OriginTyping {
		d.lastType = now
		d.lastAt = caretAfter(edits)
		d.grouped = true
	} else {
		d.grouped = false
	}
	return d.snap(), nil
}

func typingAdjacent(edits []Edit, caret Offset) bool {
	if len(edits) != 1 {
		return false
	}
	e := edits[0]
	return e.Start == caret || e.End == caret
}

func caretAfter(edits []Edit) Offset {
	if len(edits) == 0 {
		return 0
	}
	// Edits are sorted. The caret ends up after the last edit's inserted text,
	// shifted by the net length change of earlier edits.
	shift := 0
	for _, e := range edits[:len(edits)-1] {
		shift += len(e.Text) - int(e.End-e.Start)
	}
	last := edits[len(edits)-1]
	return last.Start + Offset(shift+len(last.Text))
}

func (d *Doc) validate(edits []Edit) error {
	n := d.snap().Len()
	var prevEnd Offset
	for i, e := range edits {
		if e.Start < 0 || e.End < e.Start || int(e.End) > n {
			return ErrRange
		}
		if i > 0 && e.Start < prevEnd {
			return ErrOverlap
		}
		prevEnd = e.End
	}
	return nil
}

func (d *Doc) pushHist() {
	d.undo = append(d.undo, hist{
		root:    d.root,
		addLen:  len(d.add),
		version: d.version,
		dirty:   d.dirty,
	})
	if len(d.undo) > 500 {
		d.undo = d.undo[len(d.undo)-500:]
	}
}

func (d *Doc) Undo() bool {
	if len(d.undo) == 0 {
		return false
	}
	d.redo = append(d.redo, hist{root: d.root, addLen: len(d.add), version: d.version, dirty: d.dirty})
	h := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	d.restore(h)
	return true
}

func (d *Doc) Redo() bool {
	if len(d.redo) == 0 {
		return false
	}
	d.undo = append(d.undo, hist{root: d.root, addLen: len(d.add), version: d.version, dirty: d.dirty})
	h := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	d.restore(h)
	return true
}

func (d *Doc) restore(h hist) {
	d.root = h.root
	if h.addLen <= len(d.add) {
		// Keep the backing array. Pieces only reference bytes that still exist.
		// The length is not truncated, so redo pieces that point past addLen
		// remain valid. Bytes past the restored tree are unused until redo.
	}
	d.version = h.version
	d.dirty = h.dirty
	d.grouped = false
	d.lastOrig = OriginCommand
}

// ExportForSave copies the bytes to write. It does no file IO.
// version is the document version those bytes belong to.
func (d *Doc) ExportForSave() (path string, data []byte, version int32, err error) {
	if d.path == "" {
		return "", nil, 0, ErrNoPath
	}
	if d.readOnly {
		return "", nil, 0, ErrReadOnly
	}
	b := d.snap().Bytes()
	if d.crlf {
		b = toCRLF(b)
	}
	return d.path, b, d.version, nil
}

// MarkSaved clears the dirty flag when the document is still at version.
func (d *Doc) MarkSaved(version int32) bool {
	if d.version != version {
		return false
	}
	d.dirty = false
	return true
}

// Save writes the document. CRLF files are converted back on the way out.
// An existing regular file keeps its mode.
func (d *Doc) Save(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, b, version, err := d.ExportForSave()
	if err != nil {
		return err
	}
	if err := WriteAtomic(path, b); err != nil {
		return err
	}
	d.MarkSaved(version)
	return nil
}

// WriteAtomic replaces path with data.
// A regular file keeps its permission bits. A missing path is created
// with mode 0644. The bytes go to a new file in the same directory, then
// rename replaces path. A symlink at path is replaced and is not opened.
// The temporary name is random, so a planted symlink is not followed.
func WriteAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return err
	case info.Mode().IsRegular():
		mode = info.Mode().Perm()
	case info.Mode()&os.ModeSymlink != 0:
		// Replace the directory entry. Do not open the symlink.
	default:
		return fmt.Errorf("save %s: not a regular file", path)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".gio-ide-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(err error) error {
		if cerr := f.Close(); cerr != nil && !errors.Is(cerr, os.ErrClosed) {
			err = fmt.Errorf("%w (close temp: %v)", err, cerr)
		}
		if rerr := os.Remove(tmp); rerr != nil && !os.IsNotExist(rerr) {
			err = fmt.Errorf("%w (remove temp: %v)", err, rerr)
		}
		return err
	}
	if err := f.Chmod(mode); err != nil {
		return fail(err)
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		if rerr := os.Remove(tmp); rerr != nil && !os.IsNotExist(rerr) {
			return fmt.Errorf("%w (remove temp: %v)", err, rerr)
		}
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if rerr := os.Remove(tmp); rerr != nil && !os.IsNotExist(rerr) {
			return fmt.Errorf("%w (remove temp: %v)", err, rerr)
		}
		return err
	}
	return nil
}

func toCRLF(b []byte) []byte {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	if n == 0 {
		return b
	}
	out := make([]byte, 0, len(b)+n)
	for _, c := range b {
		if c == '\n' {
			out = append(out, '\r', '\n')
			continue
		}
		out = append(out, c)
	}
	return out
}

func (d *Doc) Close() { d.closed = true }

func countNL(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func countNLString(s string) int { return countNL([]byte(s)) }
