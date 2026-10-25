package sqlite

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/Clementine33/preen/backend/internal/librarypath"
	"github.com/Clementine33/preen/backend/internal/works"
)

// coverFileNames is the name set coverdelivery publishes and reads
// (adapters/filesystem coverName), in the order a tie is broken.
var coverFileNames = []string{"cover.gif", "cover.jpeg", "cover.jpg", "cover.png", "cover.webp"}

// LocalCovers finds, for one page of works, each work's cover file in the
// stored inventory with a single primary-key lookup: the candidate paths are
// derived from the copies, so nothing walks the inventory or the disk.
// The winner per work is the first copy by (library, rel_path), then name.
//
// shortcut: Version is content_rev-mtime-size. When an external replacement
// keeps the same size and mtime, even a rescan may not detect the change, so
// the version stays the same.
func (r *Repository) LocalCovers(
	ctx context.Context,
	ids []works.WorkCode,
) (map[works.WorkCode]works.LocalCover, error) {
	return loadLocalCovers(ctx, r.db, ids)
}

func loadLocalCovers(
	ctx context.Context,
	q workDetailReader,
	ids []works.WorkCode,
) (map[works.WorkCode]works.LocalCover, error) {
	out := map[works.WorkCode]works.LocalCover{}
	if len(ids) == 0 {
		return out, nil
	}
	rawIDs := make([]string, len(ids))
	for i, id := range ids {
		rawIDs[i] = string(id)
	}
	encodedIDs, _ := json.Marshal(rawIDs)
	rows, err := q.QueryContext(ctx, `
		SELECT wc.work_id, wc.id, wc.rel_path, l.root_path
		FROM work_copies wc JOIN libraries l ON l.id = wc.library_id
		WHERE wc.work_id IN (SELECT value FROM json_each(?))
		ORDER BY wc.work_id, wc.library_id, wc.rel_path`, string(encodedIDs))
	if err != nil {
		return nil, err
	}
	type copyRef struct{ work, id, rel, member string }
	var copies []copyRef
	var candidates [][2]string
	for rows.Next() {
		var c copyRef
		var root string
		if err = rows.Scan(&c.work, &c.id, &c.rel, &root); err != nil {
			rows.Close()
			return nil, err
		}
		c.member = librarypath.NormalizeToPOSIX(librarypath.JoinRel(root, c.rel))
		copies = append(copies, c)
		for _, name := range coverFileNames {
			candidates = append(candidates, [2]string{c.member, name})
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(candidates) == 0 {
		return out, nil
	}
	encoded, _ := json.Marshal(candidates)
	files, err := q.QueryContext(ctx, `
		SELECT d.path, f.name, f.size, f.mtime, f.content_rev
		FROM json_each(?) candidate
		JOIN inventory_dirs d ON d.path = json_extract(candidate.value, '$[0]')
		JOIN inventory_files f ON f.dir_ref = d.id AND f.name = json_extract(candidate.value, '$[1]')`, string(encoded))
	if err != nil {
		return nil, err
	}
	defer files.Close()
	versions := map[string]string{}
	for files.Next() {
		var dir, name string
		var size, mtime, rev int64
		if err = files.Scan(&dir, &name, &size, &mtime, &rev); err != nil {
			return nil, err
		}
		versions[joinInventoryPath(dir, name)] = strconv.FormatInt(
			rev,
			10,
		) + "-" + strconv.FormatInt(
			mtime,
			10,
		) + "-" + strconv.FormatInt(
			size,
			10,
		)
	}
	if err = files.Err(); err != nil {
		return nil, err
	}
	for _, c := range copies {
		if _, done := out[works.WorkCode(c.work)]; done {
			continue
		}
		for _, name := range coverFileNames {
			if v, ok := versions[c.member+"/"+name]; ok {
				out[works.WorkCode(c.work)] = works.LocalCover{CopyID: c.id, RelPath: name, Version: v}
				break
			}
		}
	}
	return out, nil
}
