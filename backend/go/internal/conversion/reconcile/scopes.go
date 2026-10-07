package reconcile

import (
	"fmt"
	"strings"
)

// ValidateOtherTags rejects path rules: isolation matches individual directory
// names, so separators are malformed rather than an incomplete editing state.
func ValidateOtherTags(tags []string) error {
	for _, tag := range tags {
		if strings.ContainsAny(tag, `/\`) {
			return fmt.Errorf("other tag %q must not contain path separators", tag)
		}
	}
	return nil
}

// scopeEntries assigns every audio path to the outermost matching directory.
// Matching roots remain separate even when they match the same literal tag.
func scopeEntries(root string, entries []AudioEntry, tags []string) (map[string][]AudioEntry, error) {
	var classifier Classifier
	tags = NormalizeTags(tags)
	if len(tags) > 0 {
		var err error
		classifier, err = NewTagClassifier(tags)
		if err != nil {
			return nil, err
		}
	}
	out := map[string][]AudioEntry{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.PathPosix, root+"/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(entry.PathPosix, root+"/"), "/")
		scope := ""
		for i, dir := range parts[:len(parts)-1] {
			if classifier.Classify(dir) == PartitionMatched {
				scope = strings.Join(parts[:i+1], "/")
				break
			}
		}
		out[scope] = append(out[scope], entry)
	}
	return out, nil
}
