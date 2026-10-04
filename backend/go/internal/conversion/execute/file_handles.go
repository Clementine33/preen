package execute

import "os"

// FileHandles opens independent handles confined to the member root. The
// execution worker owns and closes them; its frozen-fact predicate judges them.
type FileHandles interface {
	// OpenSource also reopens a closed stage read-only before its hash and final
	// facts are captured.
	OpenSource(root, source string) (*os.File, error)
	CreateStage(root, stage string) (*os.File, error)
}
