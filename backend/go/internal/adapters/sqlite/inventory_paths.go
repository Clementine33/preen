package sqlite

import "strings"

// Stored inventory paths use POSIX separators on every host. Keep drive roots
// and UNC prefixes intact instead of applying the host filesystem's rules.
func splitInventoryPath(p string) (dir, name string) {
	dir, name, ok := strings.CutLast(p, "/")
	if !ok {
		return ".", p
	}
	driveRoot := len(dir) == 2 && dir[1] == ':' &&
		((dir[0] >= 'A' && dir[0] <= 'Z') || (dir[0] >= 'a' && dir[0] <= 'z'))
	if dir == "" || driveRoot {
		dir += "/"
	}
	return dir, name
}

func joinInventoryPath(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// subtree bounds strict descendants in binary order: '/' is followed by '0'.
// The path itself is matched separately when a query includes its scope root.
func subtree(p string) (from, to string) {
	p = strings.TrimSuffix(p, "/")
	return p + "/", p + "0"
}

// subtreeSQL is the SQL counterpart of subtree; expressions are adapter-owned.
// Binary comparison keeps case-distinct siblings and literal '%'/'_' separate.
func subtreeSQL(path, root string) string {
	prefix := "rtrim(" + root + ", '/')"
	return "(" + path + " >= " + prefix + " || '/' AND " + path + " < " + prefix + " || '0')"
}
