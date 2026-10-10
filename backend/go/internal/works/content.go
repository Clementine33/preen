package works

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"facette.io/natsort"

	"github.com/onsei/organizer/backend/internal/fileops"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/library"
	"github.com/onsei/organizer/backend/internal/librarypath"
)

// ==================== content manifest (ADR 0016 §2) ====================
// One work's files as the detail page reads them: every bound copy's tree,
// classified by kind and carrying the scan-time media facts. Built from the
// stored inventory — never a live walk — and deliberately not shared with the
// workbench's MemberTree: different projection, different consumers.

// ContentFileKind classifies one file for the page's deck, bay and leaf.
type ContentFileKind string

const (
	KindAudio   ContentFileKind = "aud"
	KindImage   ContentFileKind = "img"
	KindText    ContentFileKind = "txt"
	KindLyrics  ContentFileKind = "lrc"
	KindUnknown ContentFileKind = "bin"
)

// ContentNode is one node of a copy's tree: a directory carrying children, or
// a file carrying its facts. The kind field is the file discriminator, so a
// directory carries none and a file carries no children — the shapes the
// frontend union narrows on. The shapes are marshaled exactly as that: a
// directory always carries children (empty included — an empty directory is a
// fact the file browser shows) and a file always carries size, nulls
// included (unmeasured is a value, not a missing key).
type ContentNode struct {
	Name        string
	RelPath     string
	Children    []ContentNode
	Kind        ContentFileKind
	Size        int64
	Format      *string
	Bitrate     *int32
	Width       *int
	Height      *int
	DurationSec *float64
	// ThumbV is an image's thumbnail version (ADR 0015 §3): the token its
	// immutable thumbnail address carries. Empty for every other kind.
	ThumbV string
}

// MarshalJSON renders the two node shapes the contract distinguishes.
func (n ContentNode) MarshalJSON() ([]byte, error) {
	if n.Kind == "" {
		children := n.Children
		if children == nil {
			children = []ContentNode{}
		}
		return json.Marshal(dirNodeJSON{Name: n.Name, RelPath: n.RelPath, Children: children})
	}
	return json.Marshal(fileNodeJSON{
		Name: n.Name, RelPath: n.RelPath, Kind: n.Kind, Size: n.Size,
		Format: n.Format, Bitrate: n.Bitrate, Width: n.Width, Height: n.Height,
		DurationSec: n.DurationSec, ThumbV: optional(n.ThumbV),
	})
}

type dirNodeJSON struct {
	Name     string        `json:"name"`
	RelPath  string        `json:"rel_path"`
	Children []ContentNode `json:"children"`
}

type fileNodeJSON struct {
	Name        string          `json:"name"`
	RelPath     string          `json:"rel_path"`
	Kind        ContentFileKind `json:"kind"`
	Size        int64           `json:"size"`
	Format      *string         `json:"format"`
	Bitrate     *int32          `json:"bitrate"`
	Width       *int            `json:"width"`
	Height      *int            `json:"height"`
	DurationSec *float64        `json:"duration_sec"`
	ThumbV      *string         `json:"thumb_v"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ContentCopy is one bound directory's manifest: its identities plus, when the
// member is currently on disk, its tree.
type ContentCopy struct {
	CopyID    string `json:"copy_id"`
	LibraryID string `json:"library_id"`
	// DirID is the navigation identity of the copy's root directory
	// (ADR 0003), resolved here so the caller never re-derives it.
	DirID  string       `json:"dir_id"`
	Online bool         `json:"online"`
	Tree   *ContentNode `json:"tree,omitempty"`
}

// Content is the manifest of one work. Placeholder works answer with an
// empty copy list, not a 404: they are reachable, they simply have no files.
type Content struct {
	RJ     string        `json:"rj"`
	Copies []ContentCopy `json:"copies"`
}

// CopyRoot is one bound copy joined to its library's root path: what the
// manifest builds the copy's tree from.
type CopyRoot struct {
	CopyID    string
	LibraryID string
	RelPath   string
	RootPath  string
}

// ContentEntries is the inventory read side of the manifest: the entries
// stored under one member directory, and the single row the file endpoint
// serves from. The entries table is the whitelist — a path with no row is
// not served, whatever the disk holds (ADR 0016 §3).
type ContentEntries interface {
	ListEntriesUnderPath(path string) ([]inventory.Entry, error)
	EntryByPath(path string) (inventory.Entry, bool, error)
}

// ResolveMember validates one library-relative member path on disk, the seam
// direct file management owns. A copy whose member no longer resolves is
// online: false and carries no tree.
type ResolveMember func(rootPath, memberRel string) (string, error)

// ResolveMemberFile validates one member-relative item path under the same
// containment checks an operation's target goes through (ADR 0002 §1). It is
// the seam the file endpoint resolves a manifest path through.
type ResolveMemberFile func(rootPath, memberRel, rel string) (string, error)

// ContentFileLocation is one manifest file resolved for byte serving: the
// validated absolute path, the facts the ETag keys on, and the kind that
// picks the content type.
type ContentFileLocation struct {
	AbsPath    string
	Kind       ContentFileKind
	Size       int64
	Mtime      int64
	ContentRev int
	// ThumbV is the current thumbnail version of an image, the value a
	// request's v must equal for its answer to be immutable (ADR 0015 §3).
	ThumbV string
}

// thumbVersion names one image's thumbnail generation: the file facts the
// inventory rewrites when the bytes change, plus the engine that renders it.
// A change to either yields a new address, so an old one is never reused.
func thumbVersion(e *inventory.Entry, engine string) string {
	key := strconv.FormatInt(e.Size, 10) + ":" + strconv.FormatInt(e.Mtime, 10) + ":" +
		strconv.Itoa(e.ContentRev) + ":" + engine
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

// classifyContentKind names a file's kind from its extension and stored
// format. Extensions decide the text kinds first: timed text carries no MIME type
// the scan would have recorded.
func classifyContentKind(name, format string) ContentFileKind {
	switch strings.ToLower(extOf(name)) {
	case ".lrc", ".vtt":
		return KindLyrics
	case ".txt":
		return KindText
	}
	switch {
	case strings.HasPrefix(format, "audio/"):
		return KindAudio
	case strings.HasPrefix(format, "image/"):
		return KindImage
	}
	return KindUnknown
}

// containerName is the audio container an extension names, the form the
// track list aggregates by (MP3, WAV, …). Non-audio files carry none.
func containerName(kind ContentFileKind, name string) *string {
	if kind != KindAudio {
		return nil
	}
	container := strings.ToUpper(strings.TrimPrefix(extOf(name), "."))
	if container == "" {
		return nil
	}
	return &container
}

func extOf(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx:]
	}
	return ""
}

// contentBuilder assembles the tree with pointer children so a grandchild
// attached after its parent was linked still lands; the flatten step renders
// the value tree the JSON contract wants.
type contentBuilder struct {
	node     ContentNode
	children []*contentBuilder
}

// BuildContentTree assembles one copy's tree from the entries under its
// member root. Directories sort before files; within the same kind, both
// sort naturally by name — the same order the workbench's member tree uses.
// thumbEngine is the thumbnail engine's version, folded into each image's
// ThumbV.
func BuildContentTree(memberPath string, entries []inventory.Entry, thumbEngine string) *ContentNode {
	memberKey := librarypath.NormalizeToPOSIX(memberPath)
	index := map[string]*contentBuilder{
		memberKey: {node: ContentNode{Name: lastSegment(memberPath)}},
	}
	root := index[memberKey]

	for i := range entries {
		e := &entries[i]
		// The member root's own entry is already the root node.
		key := librarypath.NormalizeToPOSIX(e.Path)
		if _, exists := index[key]; exists {
			continue
		}
		b := &contentBuilder{node: ContentNode{Name: e.Name, RelPath: relativeTo(memberKey, key)}}
		if !e.IsDir {
			b.node.Kind = classifyContentKind(e.Name, e.Format)
			b.node.Size = e.Size
			b.node.Format = containerName(b.node.Kind, e.Name)
			b.node.Bitrate = e.Bitrate
			b.node.Width, b.node.Height = e.Width, e.Height
			b.node.DurationSec = e.DurationSec
			if b.node.Kind == KindImage {
				b.node.ThumbV = thumbVersion(e, thumbEngine)
			}
		}
		index[key] = b
		// Entries are ordered by path, so a parent always precedes its children.
		if parent, ok := index[librarypath.NormalizeToPOSIX(e.ParentPath)]; ok {
			parent.children = append(parent.children, b)
		}
	}

	for _, b := range index {
		children := b.children
		sort.SliceStable(children, func(i, j int) bool {
			left, right := children[i], children[j]
			leftDir, rightDir := left.node.Kind == "", right.node.Kind == ""
			if leftDir != rightDir {
				return leftDir
			}
			return natsort.Compare(left.node.Name, right.node.Name)
		})
	}
	return flattenContent(root)
}

// flattenContent renders the builder tree as nested value nodes. Directories
// keep a non-nil children slice so an empty directory still answers one.
func flattenContent(b *contentBuilder) *ContentNode {
	out := &b.node
	if b.node.Kind == "" {
		out.Children = make([]ContentNode, 0, len(b.children))
		for _, child := range b.children {
			out.Children = append(out.Children, *flattenContent(child))
		}
	}
	return out
}

// relativeTo renders an absolute entry path as member-relative POSIX. The
// member key is already normalized.
func relativeTo(memberKey, entryKey string) string {
	return strings.TrimPrefix(entryKey, strings.TrimSuffix(memberKey, "/")+"/")
}

func lastSegment(p string) string {
	if _, name, found := strings.CutLast(p, "/"); found {
		return name
	}
	return p
}

// ContentFile locates one file of the manifest for byte serving (ADR 0016
// §3). The copy must be bound to the named work, the path must resolve
// inside its member the way an operation target does (ADR 0002 §1), and the
// stored inventory must hold the row — the three checks that make the
// endpoint read-only on exactly the manifest.
func (s *Service) ContentFile(id WorkCode, copyID, relPath string) (ContentFileLocation, *Error) {
	roots, found, err := s.store.WorkCopyRoots(id)
	if err != nil {
		return ContentFileLocation{}, asInternal(err)
	}
	if !found {
		return ContentFileLocation{}, NotFound(CodeWorkNotFound, "no such work")
	}
	var root *CopyRoot
	for i := range roots {
		if roots[i].CopyID == copyID {
			root = &roots[i]
			break
		}
	}
	if root == nil {
		return ContentFileLocation{}, NotFound(CodeWorkNotFound, "no such copy for this work")
	}

	// The manifest path is member-relative; the stored row is keyed by the
	// POSIX form under the member root.
	memberAbs, resolveErr := s.resolveMember(root.RootPath, root.RelPath)
	if resolveErr != nil {
		return ContentFileLocation{}, NotFound(CodeWorkNotFound, "the copy is not on disk")
	}
	absPath, resolveErr := s.resolveMemberFile(root.RootPath, root.RelPath, relPath)
	if resolveErr != nil {
		return ContentFileLocation{}, pathRefusal(resolveErr)
	}
	entry, listed, err := s.entries.EntryByPath(librarypath.NormalizeToPOSIX(memberAbs) + "/" + relPath)
	if err != nil {
		return ContentFileLocation{}, asInternal(err)
	}
	if !listed || entry.IsDir {
		return ContentFileLocation{}, NotFound(CodeWorkNotFound, "the path is not in the work's manifest")
	}
	loc := ContentFileLocation{
		AbsPath:    absPath,
		Kind:       classifyContentKind(entry.Name, entry.Format),
		Size:       entry.Size,
		Mtime:      entry.Mtime,
		ContentRev: entry.ContentRev,
	}
	if loc.Kind == KindImage {
		loc.ThumbV = thumbVersion(&entry, s.thumbEngine)
	}
	return loc, nil
}

// pathRefusal maps a containment refusal (fileops.PathError, the one seam
// every file path validation answers with) to a catalog refusal: an
// unreadable path is reported, never guessed past.
func pathRefusal(err error) *Error {
	if pathErr, ok := errors.AsType[*fileops.PathError](err); ok {
		if pathErr.Code == fileops.CodePathInvalid {
			return Invalid(CodeInvalidArgument, pathErr.Message)
		}
		return NotFound(CodeWorkNotFound, pathErr.Message)
	}
	return asInternal(err)
}

// Content answers one work's file manifest (ADR 0016 §2). The trees come from
// the stored inventory; a copy whose member root no longer resolves on disk
// is answered online: false with no tree rather than a stale one.
func (s *Service) Content(id WorkCode) (*Content, *Error) {
	roots, found, err := s.store.WorkCopyRoots(id)
	if err != nil {
		return nil, asInternal(err)
	}
	if !found {
		return nil, NotFound(CodeWorkNotFound, "no such work")
	}
	copies := make([]ContentCopy, 0, len(roots))
	for _, root := range roots {
		cp := ContentCopy{
			CopyID:    root.CopyID,
			LibraryID: root.LibraryID,
			DirID:     library.DirID(root.LibraryID, root.RootPath, root.RelPath),
		}
		memberAbs, resolveErr := s.resolveMember(root.RootPath, root.RelPath)
		if resolveErr == nil {
			cp.Online = true
			entries, listErr := s.entries.ListEntriesUnderPath(memberAbs)
			if listErr != nil {
				return nil, asInternal(listErr)
			}
			cp.Tree = BuildContentTree(memberAbs, entries, s.thumbEngine)
		}
		copies = append(copies, cp)
	}
	return &Content{RJ: string(id), Copies: copies}, nil
}
