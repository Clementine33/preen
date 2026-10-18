package refresh_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

// batchCatalog keeps catalog I/O outside the scheduling seam under test.
type batchCatalog struct{}

func (batchCatalog) Detail(works.WorkCode) (*works.WorkDetail, *works.Error) {
	return &works.WorkDetail{}, nil
}
func (batchCatalog) Import(works.WorkCode, works.ImportRequest) (works.ImportOutcome, *works.Error) {
	panic("cancelled refresh imported")
}

func (batchCatalog) MetadataStates(
	context.Context,
	[]works.WorkCode,
) (map[works.WorkCode]map[works.DataGroup]works.SourceState, error) {
	return map[works.WorkCode]map[works.DataGroup]works.SourceState{}, nil
}

type cancelBatch struct {
	cancel  context.CancelFunc
	batches [][]works.WorkCode
}

func (s *cancelBatch) FetchDynamic(
	ctx context.Context,
	ids []works.WorkCode,
	_ string,
) (map[works.WorkCode]works.ImportRequest, error) {
	s.batches = append(s.batches, ids)
	s.cancel()
	return nil, ctx.Err()
}
func (*cancelBatch) FetchStatic(context.Context, works.WorkCode, string) (refresh.Snapshot, error) {
	panic("cancelled refresh fetched static")
}
func (*cancelBatch) FetchASMR(context.Context, works.WorkCode, string) (refresh.Snapshot, error) {
	panic("cancelled refresh fetched backup")
}

func TestCancellationStopsBeforeTheNextDynamicBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &cancelBatch{cancel: cancel}
	codes := make([]string, 501)
	for i := range codes {
		codes[i] = fmt.Sprintf("RJ%08d", 501-i)
	}
	_, err := refresh.NewService(batchCatalog{}, batchCatalog{}, source, source).
		Refresh(ctx, refresh.Request{WorkCodes: codes, Mode: refresh.RefreshDynamic})
	if err == nil || len(source.batches) != 1 || len(source.batches[0]) != 500 || source.batches[0][0] != "RJ00000001" {
		t.Fatalf("batch cancellation: %d batches, err=%v", len(source.batches), err)
	}
}
