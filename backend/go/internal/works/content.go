package works

import (
	"encoding/json"
	"sort"
	"strings"

	"facette.io/natsort"

	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/library"
	"github.com/onsei/organizer/backend/internal/pathnorm"
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
		DurationSec: n.DurationSec,
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
// stored under one member directory, the same read the library's own browsing
// goes through.
type ContentEntries interface {
	ListEntriesUnderPath(path string) ([]inventory.Entry, error)
}

// ResolveMember validates one library-relative member path on disk, the seam
// direct file management owns. A copy whose member no longer resolves is
// online: false and carries no tree.
type ResolveMember func(rootPath, memberRel string) (string, error)

// classifyContentKind names a file's kind from its extension and stored
// format. Extensions decide the text kinds first: .lrc carries no MIME type
// the scan would have recorded.
func classifyContentKind(name, format string) ContentFileKind {
	switch strings.ToLower(extOf(name)) {
	case ".lrc":
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
func BuildContentTree(memberPath string, entries []inventory.Entry) *ContentNode {
	memberKey := pathnorm.NormalizeToPOSIX(memberPath)
	index := map[string]*contentBuilder{
		memberKey: {node: ContentNode{Name: lastSegment(memberPath)}},
	}
	root := index[memberKey]

	for i := range entries {
		e := &entries[i]
		// The member root's own entry is already the root node.
		key := pathnorm.NormalizeToPOSIX(e.Path)
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
		}
		index[key] = b
		// Entries are ordered by path, so a parent always precedes its children.
		if parent, ok := index[pathnorm.NormalizeToPOSIX(e.ParentPath)]; ok {
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
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
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
			cp.Tree = BuildContentTree(memberAbs, entries)
		}
		copies = append(copies, cp)
	}
	return &Content{RJ: string(id), Copies: copies}, nil
}
