package execute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

// CompareFrozenFacts is shared by the path precheck and the live source handle.
// Unspecified legacy fields impose no constraint; time retains its ±1s tolerance.
func CompareFrozenFacts(tuple reconcile.FileTuple, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}
	if tuple.Size > 0 && info.Size() != tuple.Size {
		return fmt.Errorf("size changed: frozen %d, disk %d", tuple.Size, info.Size())
	}
	if tuple.Mtime > 0 {
		delta := info.ModTime().Sub(time.Unix(tuple.Mtime, 0))
		if delta < -time.Second || delta > time.Second {
			return fmt.Errorf("mtime changed: frozen %d, disk %d", tuple.Mtime, info.ModTime().Unix())
		}
	}
	return nil
}

// OutputFacts were captured from the encoded object before publication.
type OutputFacts struct {
	Path   string
	Size   int64
	Mtime  int64
	SHA256 string
	Spec   reconcile.AudioOutputSpec
}

func sameArtifact(expected, actual os.FileInfo) bool {
	return expected != nil && actual.Mode().IsRegular() && os.SameFile(expected, actual) &&
		expected.Size() == actual.Size() &&
		expected.ModTime().Equal(actual.ModTime())
}

func checkStagedIdentity(enc *plannedEncode) error {
	info, err := os.Lstat(enc.temp)
	if err != nil {
		return err
	}
	if !sameArtifact(enc.artifact, info) {
		return fmt.Errorf("staged output identity or facts changed")
	}
	return nil
}

func (p *plannedComponent) checkRuntimeInput(native string) error {
	if err := p.checkPath(native); err != nil {
		return err
	}
	info, err := os.Lstat(native)
	if err != nil {
		return err
	}
	if !os.SameFile(p.inputs[posixForm(native)], info) {
		return fmt.Errorf("input identity changed")
	}
	return CompareFrozenFacts(p.frozen[posixForm(native)], info)
}

func (p *PreparedComponent) materialize(ctx context.Context, enc *plannedEncode) (failure error) {
	if err := p.plan.checkPath(enc.source); err != nil {
		return err
	}
	if p.files == nil {
		return fmt.Errorf("file handle capability unavailable")
	}
	source, err := p.files.OpenSource(p.plan.absRoot, enc.source)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, source.Close()) }()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(p.plan.inputs[posixForm(enc.source)], info) {
		return fmt.Errorf("source identity changed")
	}
	if factsErr := CompareFrozenFacts(p.plan.frozen[posixForm(enc.source)], info); factsErr != nil {
		return factsErr
	}
	if err := p.plan.checkPath(enc.temp); err != nil {
		return err
	}
	output, err := p.files.CreateStage(p.plan.absRoot, enc.temp)
	if err != nil {
		enc.temp = "" // O_EXCL failed: this name never belonged to this task.
		return err
	}
	defer func() {
		if output != nil {
			failure = errors.Join(failure, output.Close())
		}
	}()
	enc.artifact, err = output.Stat()
	if err != nil {
		enc.protected = true
		return err
	}
	if _, encodeErr := p.tk.encode(ctx, source, output, enc.spec); encodeErr != nil {
		return encodeErr
	}
	written, err := output.Stat()
	if err != nil {
		return err
	}
	// SMB may finalize mtime when the last writable handle closes, even after
	// Sync. Close it before hashing and freezing the facts used by both publish
	// checks. The read-only reopen must still identify the same encoded object.
	writer := output
	output = nil
	if closeErr := writer.Close(); closeErr != nil {
		return closeErr
	}
	output, err = p.files.OpenSource(p.plan.absRoot, enc.temp)
	if err != nil {
		return err
	}
	before, err := output.Stat()
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || !os.SameFile(written, before) || written.Size() != before.Size() {
		return fmt.Errorf("output identity or size changed while closing writer")
	}
	if !enc.spec.Lossless() {
		if _, seekErr := output.Seek(0, 0); seekErr != nil {
			return seekErr
		}
		hash := sha256.New()
		if _, hashErr := io.Copy(hash, output); hashErr != nil {
			return hashErr
		}
		enc.hash = hex.EncodeToString(hash.Sum(nil))
	}
	after, err := output.Stat()
	if err != nil {
		return err
	}
	if !sameArtifact(before, after) {
		return fmt.Errorf("output changed during hashing")
	}
	enc.artifact = after // File.Stat materializes identity from the open handle.
	return nil
}
