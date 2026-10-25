package coverdelivery_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

type download struct {
	data  []byte
	calls int
	urls  []string
	fn    func(context.Context, string) (coverdelivery.Download, error)
}

func (d *download) Download(ctx context.Context, url string) (coverdelivery.Download, error) {
	d.calls++
	d.urls = append(d.urls, url)
	if d.fn != nil {
		return d.fn(ctx, url)
	}
	return coverdelivery.Download{Data: d.data, Extension: "png", ContentType: "image/png"}, nil
}
func TestDeliveryDownloadsOnceAndPublishesSelectedCopies(t *testing.T) {
	svc, _, dl, req, folders := deliveryFixture(t)
	var b bytes.Buffer
	b.Write(dl.data)
	var err error
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 1 {
		t.Fatalf("delivery: %+v, %v, downloads %d", report, err, dl.calls)
	}
	for _, dir := range folders {
		data, e := os.ReadFile(filepath.Join(dir, "cover.png"))
		if e != nil || !bytes.Equal(data, b.Bytes()) {
			t.Fatalf("missing published file: %v", e)
		}
	}
}

func TestRetiredCoverDemandCannotPublishAfterDownload(t *testing.T) {
	svc, repo, dl, req, folders := deliveryFixture(t)
	image := dl.data
	dl.fn = func(_ context.Context, _ string) (coverdelivery.Download, error) {
		if _, err := repo.DB().Exec(`DELETE FROM plans WHERE plan_id='plan'`); err != nil {
			return coverdelivery.Download{}, err
		}
		return coverdelivery.Download{Data: image, Extension: "png", ContentType: "image/png"}, nil
	}
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, folder := range folders {
		if _, err := os.Stat(filepath.Join(folder, "cover.png")); !os.IsNotExist(err) {
			t.Fatalf("retired demand published a cover: %v", err)
		}
	}
}

func deliveryFixture(
	t *testing.T,
) (*coverdelivery.Service, *sqlite.Repository, *download, coverdelivery.Request, []string) {
	t.Helper()
	root := t.TempDir()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	lib, err := repo.CreateLibrary("lib", root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.DB().
		Exec(`INSERT INTO worksets(id) VALUES('set');INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('plan','/music','s','set');INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec','set','plan');`); err != nil {
		t.Fatal(err)
	}
	folders := []string{}
	for _, name := range []string{"RJ00000001 a", "RJ00000001 b"} {
		dir := filepath.Join(root, name)
		if err = os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		folders = append(folders, filepath.ToSlash(dir))
		inventoryfixture.SeedInventory(t, repo, inventoryfixture.Row{
			Path:       filepath.ToSlash(dir),
			RootPath:   filepath.ToSlash(root),
			IsDir:      true,
			IsWorkRoot: true,
		})
		if _, err = repo.DB().
			Exec(`INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000001','now','now') ON CONFLICT(id) DO NOTHING`); err != nil {
			t.Fatal(err)
		}
		if _, err = repo.DB().
			Exec(`INSERT INTO work_copies(id,work_id,library_id,rel_path,created_at) VALUES(?,'RJ00000001',?,?,'now')`, strings.ReplaceAll(name, " ", "-"), lib.ID, name); err != nil {
			t.Fatal(err)
		}
	}

	setCoverURL(t, repo, "https://img.dlsite.jp/main")
	targets, err := repo.CoverTargets(t.Context(), root, folders)
	if err != nil {
		t.Fatal(err)
	}
	req := coverdelivery.Request{
		ExecutionID: "exec",
		WorkCode:    "RJ00000001",
		Targets:     []coverdelivery.Target{},
	}
	for _, dir := range folders {
		target := targets[dir]
		target.Identity = "legacy-identity"
		req.Targets = append(req.Targets, target)
	}
	var b bytes.Buffer
	if err = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	dl := &download{data: b.Bytes()}
	gate := admission.NewGate(nil)
	svc, err := coverdelivery.New(
		coverdelivery.Dependencies{
			Store:      repo,
			Downloader: dl,
			Files:      filesystem.CoverFiles{},
			Acquire:    gate.BeginManual,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, dl, req, folders
}

func TestCancellationStopsUnusedDownloadAndExplicitRetryRestarts(t *testing.T) {
	svc, _, dl, req, folders := deliveryFixture(t)
	dl.fn = func(ctx context.Context, _ string) (coverdelivery.Download, error) {
		if err := svc.Cancel(t.Context(), "exec"); err != nil {
			t.Fatal(err)
		}
		if ctx.Err() == nil {
			t.Error("unused shared download was not canceled")
		}
		return coverdelivery.Download{}, ctx.Err()
	}
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if worked, err := svc.ProcessNext(t.Context()); err != nil || worked {
		t.Fatalf("stopped request resumed: %v %v", worked, err)
	}
	for _, folder := range folders {
		if _, err := os.Stat(filepath.Join(folder, "cover.png")); !os.IsNotExist(err) {
			t.Fatal("cancel wrote cover")
		}
	}
	dl.fn = nil
	if err := svc.Retry(t.Context(), "exec"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 {
		t.Fatalf("retry: %+v %v", report, err)
	}
}
func TestCanceledDemandDoesNotStopOtherExecution(t *testing.T) {
	svc, repo, dl, req, folders := deliveryFixture(t)
	if _, err := repo.DB().Exec(`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id)
		VALUES('other-plan','/music','s','set')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().
		Exec(`INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('other','set','other-plan')`); err != nil {
		t.Fatal(err)
	}
	other := req
	other.ExecutionID = "other"
	other.Targets = req.Targets[1:]
	req.Targets = req.Targets[:1]
	for _, r := range []coverdelivery.Request{req, other} {
		if err := svc.Enqueue(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	dl.fn = func(ctx context.Context, _ string) (coverdelivery.Download, error) {
		if err := svc.Cancel(t.Context(), "exec"); err != nil {
			t.Fatal(err)
		}
		if ctx.Err() != nil {
			t.Fatal("shared download canceled while still needed")
		}
		return coverdelivery.Download{Data: dl.data}, nil
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folders[0], "cover.png")); !os.IsNotExist(err) {
		t.Fatal("canceled copy written")
	}
	if _, err := os.Stat(filepath.Join(folders[1], "cover.png")); err != nil {
		t.Fatal(err)
	}
}
func TestBindingChangeKeepsFrozenCoverTarget(t *testing.T) {
	svc, repo, dl, req, folders := deliveryFixture(t)
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().
		Exec(`UPDATE work_copies SET rel_path=rel_path||'-changed' WHERE id=?`, req.Targets[0].CopyID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 1 {
		t.Fatalf("copies: %+v %v", report, err)
	}
	if _, err = os.Stat(filepath.Join(folders[0], "cover.png")); err != nil {
		t.Fatal("frozen copy target was not written", err)
	}
}
func TestInventoryFailureKeepsPublicationAndLocalRead(t *testing.T) {
	_, repo, dl, req, _ := deliveryFixture(t)
	svc, err := coverdelivery.New(
		coverdelivery.Dependencies{
			Store:      repo,
			Downloader: dl,
			Files:      filesystem.CoverFiles{},
			Acquire:    admission.NewGate(nil).BeginManual,
			SyncInventory: func(context.Context, coverdelivery.Target, coverdelivery.Publication) error {
				return fmt.Errorf("inventory unavailable")
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["failed"] != 2 || report.Items[0].ErrorCode != "INVENTORY_REFRESH_FAILED" ||
		report.Items[0].Published != "cover.png" {
		t.Fatalf("publication not disclosed: %+v %v", report, err)
	}
	handler := httpapi.NewServer(httpapi.Dependencies{Token: "token", Covers: svc})
	path := "/api/v1/works/RJ00000001/copies/" + req.Targets[0].CopyID + "/cover"
	for _, tc := range []struct {
		path, token string
		status      int
	}{{path, "", 401}, {path, "token", 200}, {"/api/v1/works/RJ00000002/copies/" + req.Targets[0].CopyID + "/cover", "token", 404}} {
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		request.Header.Set("Authorization", "Bearer "+tc.token)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, request)
		if out.Code != tc.status {
			t.Fatalf("read: %d %s", out.Code, out.Body.String())
		}
		if tc.status == 200 &&
			(!bytes.Equal(out.Body.Bytes(), dl.data) || out.Header().Get("Content-Type") != "image/png") {
			t.Fatal("local bytes or format incorrect")
		}
	}
	if dl.calls != 1 {
		t.Fatal("GET triggered download")
	}
}

func setCoverURL(t *testing.T, repo *sqlite.Repository, url string) {
	t.Helper()
	if _, err := repo.DB().Exec(
		`INSERT INTO works(id,cover_url,collected_at,updated_at) VALUES('RJ00000001',?,'now','now')
		 ON CONFLICT(id) DO UPDATE SET cover_url=excluded.cover_url`, url); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveriesOfOneWorkAcrossExecutionsShareOneDownload(t *testing.T) {
	svc, repo, dl, req, folders := deliveryFixture(t)
	if _, err := repo.DB().Exec(`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id)
		VALUES('other-plan','/music','s','set');
		INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('other','set','other-plan')`); err != nil {
		t.Fatal(err)
	}
	other := req
	other.ExecutionID = "other"
	other.Targets = req.Targets[1:]
	req.Targets = req.Targets[:1]
	for _, r := range []coverdelivery.Request{req, other} {
		if err := svc.Enqueue(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if worked, err := svc.ProcessNext(t.Context()); err != nil || worked {
		t.Fatalf("second claim: %v %v", worked, err)
	}
	if dl.calls != 1 {
		t.Fatalf("downloads: %d", dl.calls)
	}
	for _, folder := range folders {
		if _, err := os.Stat(filepath.Join(folder, "cover.png")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetryRequeuesEveryFailedOrCanceledDelivery(t *testing.T) {
	svc, repo, dl, req, _ := deliveryFixture(t)
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(
		`UPDATE cover_deliveries SET status=CASE copy_id WHEN ? THEN 'failed' ELSE 'canceled' END`,
		req.Targets[0].CopyID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Retry(t.Context(), "exec"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 1 {
		t.Fatalf("retry: %+v %v", report, err)
	}
}

func TestDownloadReadsCurrentWorkCoverURL(t *testing.T) {
	svc, repo, dl, req, _ := deliveryFixture(t)
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	setCoverURL(t, repo, "https://img.dlsite.jp/newer.webp")
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(dl.urls) != 1 || dl.urls[0] != "https://img.dlsite.jp/newer.webp" {
		t.Fatalf("urls: %v", dl.urls)
	}
}

type asmrSource struct {
	url   string
	calls []string
}

func (s *asmrSource) FetchMain(_ context.Context, _, provider, _ string) (string, error) {
	s.calls = append(s.calls, provider)
	return s.url, nil
}

func TestMissingOrFailedMainFallsBackToASMROne(t *testing.T) {
	for _, tc := range []struct {
		name, main string
		wantCalls  int
	}{{"empty", "", 1}, {"download fails", "https://img.dlsite.jp/broken", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			_, repo, dl, req, _ := deliveryFixture(t)
			setCoverURL(t, repo, tc.main)
			image := dl.data
			dl.fn = func(_ context.Context, url string) (coverdelivery.Download, error) {
				if url != "https://img.asmr-200.com/backup.webp" {
					return coverdelivery.Download{}, fmt.Errorf("rejected")
				}
				return coverdelivery.Download{Data: image}, nil
			}
			source := &asmrSource{url: "https://img.asmr-200.com/backup.webp"}
			svc, err := coverdelivery.New(coverdelivery.Dependencies{
				Store: repo, Downloader: dl, Source: source, Files: filesystem.CoverFiles{},
				Acquire: admission.NewGate(nil).BeginManual,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err = svc.Enqueue(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			if _, err = svc.ProcessNext(t.Context()); err != nil {
				t.Fatal(err)
			}
			report, err := svc.Report(t.Context(), "exec")
			if err != nil || report.Counts["succeeded"] != 2 || len(source.calls) != 1 ||
				source.calls[0] != "asmr_one" ||
				len(dl.urls) != tc.wantCalls {
				t.Fatalf("fallback: %+v %v source=%v urls=%v", report, err, source.calls, dl.urls)
			}
		})
	}
}

func TestRefreshRedownloadsAndSkipsIdenticalBytes(t *testing.T) {
	svc, repo, dl, req, folders := deliveryFixture(t)
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(folders[0], "cover.png"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.DB().Exec(`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id)
		VALUES('refresh-plan','/music','s','set');
		INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('refresh','set','refresh-plan')`); err != nil {
		t.Fatal(err)
	}
	req.ExecutionID = "refresh"
	req.Refresh = true
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "refresh")
	if err != nil || report.Counts["skipped"] != 2 || dl.calls != 2 {
		t.Fatalf("refresh: %+v %v calls=%d", report, err, dl.calls)
	}
	after, err := os.Stat(filepath.Join(folders[0], "cover.png"))
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("identical bytes rewritten: %v", err)
	}
}

func TestBusyAdmissionWaitsWithoutDownloadingAgain(t *testing.T) {
	_, repo, dl, req, folders := deliveryFixture(t)
	attempts := 0
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: repo, Downloader: dl, Files: filesystem.CoverFiles{}, BusyRetry: time.Millisecond,
		Acquire: func() (func(), error) {
			if attempts++; attempts <= 6 {
				return nil, &admission.BusyError{Reason: "test admission"}
			}
			return func() {}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 1 {
		t.Fatalf("busy wait: %+v %v calls=%d", report, err, dl.calls)
	}
	for _, folder := range folders {
		if _, err = os.Stat(filepath.Join(folder, "cover.png")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBusyAdmissionYieldsToRestartRecovery(t *testing.T) {
	_, repo, dl, req, _ := deliveryFixture(t)
	busy := true
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: repo, Downloader: dl, Files: filesystem.CoverFiles{}, BusyRetry: time.Millisecond,
		Acquire: func() (func(), error) {
			if busy {
				return nil, &admission.BusyError{Reason: "test admission"}
			}
			return func() {}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err = svc.ProcessNext(ctx); err == nil {
		t.Fatal("shutdown during busy wait was not reported")
	}
	// The process died with the rows claimed; restart returns them to pending.
	if err = svc.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	busy = false
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 2 {
		t.Fatalf("resume: %+v %v calls=%d", report, err, dl.calls)
	}
}
