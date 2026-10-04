package execute

import (
	"context"
	"errors"
	"fmt"
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
		expected.Size() == actual.Size()
}

func checkStagedIdentity(enc *plannedEncode) error {
	info, err := os.Lstat(enc.temp)
	if err != nil {
		return err
	}
	if !sameArtifact(enc.artifact, info) {
		return fmt.Errorf("staged output identity or size changed")
	}
	enc.artifact = info // Record the latest observed mtime without treating it as a guard.
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
	validating := false
	defer func() {
		if failure != nil && validating {
			failure = componentError(ComponentStageMaterialize, ComponentCodeValidateFailed,
				posixForm(enc.target), "staged output validation failed", failure)
		}
	}()
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
	input, encodeErr := p.tk.encode(ctx, source, output, enc.spec)
	if encodeErr != nil {
		return encodeErr
	}
	validating = true
	if syncErr := output.Sync(); syncErr != nil {
		return syncErr
	}
	written, err := output.Stat()
	if err != nil {
		return err
	}
	// Validate a read-only handle after flushing and closing the writer. Remote
	// mtime may still settle later; only identity and size guard this owned stage.
	writer := output
	output = nil
	if closeErr := writer.Close(); closeErr != nil {
		return closeErr
	}
	var before os.FileInfo
	output, before, err = p.reopenStagedOutput(enc, written)
	if err != nil {
		return err
	}
	validated, err := p.tk.validate(ctx, output, enc.spec, input)
	if err != nil {
		return err
	}
	after, err := output.Stat()
	if err != nil {
		return err
	}
	if !sameArtifact(before, after) {
		return fmt.Errorf("output identity or size changed during validation: same_file=%t, size=%d -> %d",
			os.SameFile(before, after), before.Size(), after.Size())
	}
	enc.hash, enc.artifact = validated.SHA256, after // File.Stat materializes the open object's identity.
	return nil
}

// Reopen after the writer closes. The read handle must name the encoded object
// with the written size; mtime is an observed credential fact, not a guard.
func (p *PreparedComponent) reopenStagedOutput(enc *plannedEncode, written os.FileInfo) (*os.File, os.FileInfo, error) {
	file, err := p.files.OpenSource(p.plan.absRoot, enc.temp)
	if err != nil {
		return nil, nil, err
	}
	before, err := file.Stat()
	if err == nil && (!before.Mode().IsRegular() || !os.SameFile(written, before) || written.Size() != before.Size()) {
		err = fmt.Errorf(
			"output identity or size changed while closing writer: regular=%t, same_file=%t, size=%d -> %d",
			before.Mode().IsRegular(), os.SameFile(written, before), written.Size(), before.Size(),
		)
	}
	if err != nil {
		return nil, nil, errors.Join(err, file.Close())
	}
	return file, before, nil
}
