package inventory

import (
	"fmt"
	"slices"
	"strings"

	"github.com/onsei/organizer/backend/internal/librarypath"
)

// SelectedScopes validates and collapses explicit subtrees. An empty set never
// means a whole-library scan. Root markers are preserved by selected cleanup.
func SelectedScopes(root string, folders []string) ([]string, error) {
	root = librarypath.CleanRootPath(root)
	if root == "" {
		return nil, fmt.Errorf("selected scan requires a library root")
	}
	paths := make([]string, 0, len(folders))
	for _, folder := range folders {
		folder = librarypath.CleanRootPath(folder)
		if folder == "" || folder == root || !strings.HasPrefix(folder, strings.TrimRight(root, "/")+"/") {
			return nil, fmt.Errorf("selected folder is not below library root: %s", folder)
		}
		paths = append(paths, folder)
	}
	slices.Sort(paths)
	result := make([]string, 0, len(paths))
	for _, folder := range paths {
		covered := slices.ContainsFunc(result, func(previous string) bool {
			return folder == previous || strings.HasPrefix(folder, strings.TrimRight(previous, "/")+"/")
		})
		if covered {
			continue
		}
		result = append(result, folder)
	}
	return result, nil
}
