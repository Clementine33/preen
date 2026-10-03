package covers_test

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
	"strconv"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/httpapi"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/admission"
	"github.com/onsei/organizer/backend/internal/covers"
)

type download struct {
	data  []byte
	calls int
	fn    func(context.Context, covers.Validators) (covers.Download, error)
}

func (d *download) Download(ctx context.Context, _ string, v covers.Validators) (covers.Download, error) {
	d.calls++
	if d.fn != nil {
		return d.fn(ctx, v)
	}
	return covers.Download{Data: d.data, Extension: "png", ContentType: "image/png"}, nil
}
func TestDeliveryDownloadsOnceAndPublishesSelectedCopies(t *testing.T) {
	svc, _, dl, req, folders, _ := deliveryFixture(t)
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
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 1 || report.Downloads["total"] != 1 {
		t.Fatalf("delivery: %+v, %v, downloads %d", report, err, dl.calls)
	}
	for _, dir := range folders {
		data, e := os.ReadFile(filepath.Join(dir, "cover.png"))
		if e != nil || !bytes.Equal(data, b.Bytes()) {
			t.Fatalf("missing published file: %v", e)
		}
	}
}

func deliveryFixture(t *testing.T) (*covers.Service, *sqlite.Repository, *download, covers.Request, []string, string) {
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
		Exec(`INSERT INTO worksets(id) VALUES('set');INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('exec','set','plan');`); err != nil {
		t.Fatal(err)
	}
	folders := []string{}
	for _, name := range []string{"RJ00000001 a", "RJ00000001 b"} {
		dir := filepath.Join(root, name)
		if err = os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		folders = append(folders, filepath.ToSlash(dir))
		if _, err = repo.DB().
			Exec(`INSERT INTO entries_staging(session_id,path,root_path,parent_path,name,is_dir,is_work_root,size,mtime,operation) VALUES('scan',?,?,?, ?,1,1,0,0,'upsert')`, filepath.ToSlash(dir), filepath.ToSlash(root), filepath.ToSlash(root), name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = repo.MergeStaging("scan", lib.ID, filepath.ToSlash(root)); err != nil {
		t.Fatal(err)
	}
	targets, err := repo.CoverTargets(t.Context(), root, folders)
	if err != nil {
		t.Fatal(err)
	}
	req := covers.Request{
		ExecutionID: "exec",
		WorkCode:    "RJ00000001",
		URL:         "https://img.dlsite.jp/main",
		Targets:     []covers.Target{},
	}
	for _, dir := range folders {
		target := targets[dir]
		target.Identity, err = filesystem.CoverDirectoryIdentity(dir)
		if err != nil {
			t.Fatal(err)
		}
		req.Targets = append(req.Targets, target)
	}
	var b bytes.Buffer
	if err = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	dl := &download{data: b.Bytes()}
	gate := admission.NewGate(nil)
	stage := t.TempDir()
	svc, err := covers.New(
		covers.Dependencies{
			Store:      repo,
			Downloader: dl,
			Files:      filesystem.CoverFiles{},
			StageDir:   stage,
			Acquire:    gate.BeginManual,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, dl, req, folders, stage
}

func TestBusyDeliveryResumesItsStageAfterRestart(t *testing.T) {
	svc, repo, dl, req, folders, stage := deliveryFixture(t)
	busy := true
	recovered, err := covers.New(
		covers.Dependencies{
			Store:      repo,
			Downloader: dl,
			Files:      filesystem.CoverFiles{},
			StageDir:   stage,
			Acquire: func() (func(), error) {
				if busy {
					return nil, &admission.BusyError{Reason: "test admission"}
				}
				return func() {}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = recovered.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, folder := range folders {
		if _, err = os.Stat(filepath.Join(folder, "cover.png")); !os.IsNotExist(err) {
			t.Fatal("busy gate allowed publication")
		}
	}
	if dl.calls != 1 {
		t.Fatal("download should not wait for admission")
	}
	orphan := filepath.Join(stage, "00000000-0000-0000-0000-000000000000.image")
	if err = os.WriteFile(orphan, dl.data, 0600); err != nil {
		t.Fatal(err)
	}
	var sequence int
	var name, dbPath string
	if err = repo.DB().QueryRow("PRAGMA database_list").Scan(&sequence, &name, &dbPath); err != nil {
		t.Fatal(err)
	}
	if err = repo.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.NewDevelopmentRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	recovered, err = covers.New(
		covers.Dependencies{
			Store:      reopened,
			Downloader: dl,
			Files:      filesystem.CoverFiles{},
			StageDir:   stage,
			Acquire:    admission.NewGate(nil).BeginManual,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = recovered.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("unreferenced staging image survived restart")
	}
	busy = false
	if _, err = recovered.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := recovered.Report(t.Context(), "exec")
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 1 {
		t.Fatalf("resume: %+v %v calls=%d", report, err, dl.calls)
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 0 {
		t.Fatalf("stage leaked: %v %v", entries, err)
	}
}

func TestCancellationStopsUnusedDownloadAndExplicitRetryRestarts(t *testing.T) {
	svc, _, dl, req, folders, _ := deliveryFixture(t)
	dl.fn = func(ctx context.Context, _ covers.Validators) (covers.Download, error) {
		if err := svc.Cancel(t.Context(), "exec"); err != nil {
			t.Fatal(err)
		}
		if ctx.Err() == nil {
			t.Error("unused shared download was not canceled")
		}
		return covers.Download{}, ctx.Err()
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
	svc, repo, dl, req, folders, _ := deliveryFixture(t)
	if _, err := repo.DB().
		Exec(`INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('other','set','other-plan')`); err != nil {
		t.Fatal(err)
	}
	other := req
	other.ExecutionID = "other"
	other.Targets = req.Targets[1:]
	req.Targets = req.Targets[:1]
	for _, r := range []covers.Request{req, other} {
		if err := svc.Enqueue(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	dl.fn = func(ctx context.Context, _ covers.Validators) (covers.Download, error) {
		if err := svc.Cancel(t.Context(), "exec"); err != nil {
			t.Fatal(err)
		}
		if ctx.Err() != nil {
			t.Fatal("shared download canceled while still needed")
		}
		return covers.Download{Data: dl.data}, nil
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
func TestNewSourceVersionPreventsOldPublication(t *testing.T) {
	svc, repo, dl, req, folders, _ := deliveryFixture(t)
	if _, err := repo.DB().
		Exec(`INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('new','set','new-plan')`); err != nil {
		t.Fatal(err)
	}
	newer := req
	newer.ExecutionID = "new"
	newer.URL = "https://img.dlsite.jp/new.webp"
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	dl.fn = func(_ context.Context, _ covers.Validators) (covers.Download, error) {
		if err := svc.Enqueue(t.Context(), newer); err != nil {
			t.Fatal(err)
		}
		return covers.Download{Data: dl.data}, nil
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, folder := range folders {
		if _, err := os.Stat(filepath.Join(folder, "cover.png")); !os.IsNotExist(err) {
			t.Fatal("stale source landed")
		}
	}
	old, err := svc.Report(t.Context(), "exec")
	if err != nil || old.Counts["canceled"] != 2 || old.Items[0].ErrorCode != "SOURCE_UPDATED" {
		t.Fatalf("stale: %+v %v", old, err)
	}
	dl.fn = nil
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestMissingBindingOnlyFailsItsOwnCopy(t *testing.T) {
	svc, repo, dl, req, folders, _ := deliveryFixture(t)
	if err := svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`DELETE FROM work_copies WHERE id=?`, req.Targets[0].CopyID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Report(t.Context(), "exec")
	if err != nil || report.Counts["failed"] != 1 || report.Counts["succeeded"] != 1 || dl.calls != 1 {
		t.Fatalf("copies: %+v %v", report, err)
	}
	if _, err = os.Stat(filepath.Join(folders[0], "cover.png")); !os.IsNotExist(err) {
		t.Fatal("unbound copy written")
	}
}
func TestInventoryFailureKeepsPublicationAndLocalRead(t *testing.T) {
	_, repo, dl, req, _, stage := deliveryFixture(t)
	svc, err := covers.New(
		covers.Dependencies{
			Store:      repo,
			Downloader: dl,
			Files:      filesystem.CoverFiles{},
			StageDir:   stage,
			Acquire:    admission.NewGate(nil).BeginManual,
			Refresh:    func(context.Context, string, string) error { return fmt.Errorf("inventory unavailable") },
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
func TestConditionalRefreshRequiresMatchingLocalBytes(t *testing.T) {
	for _, keep := range []bool{true, false} {
		t.Run(strconv.FormatBool(keep), func(t *testing.T) {
			svc, repo, dl, req, folders, _ := deliveryFixture(t)
			dl.fn = func(context.Context, covers.Validators) (covers.Download, error) {
				return covers.Download{Data: dl.data, ETag: "saved"}, nil
			}
			if err := svc.Enqueue(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ProcessNext(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.DB().
				Exec(`INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('refresh','set','refresh-plan')`); err != nil {
				t.Fatal(err)
			}
			if !keep {
				for _, folder := range folders {
					if err := os.Remove(filepath.Join(folder, "cover.png")); err != nil {
						t.Fatal(err)
					}
				}
			}
			req.ExecutionID = "refresh"
			req.Refresh = true
			dl.fn = conditionalDownload(t, dl.data, keep)
			if err := svc.Enqueue(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ProcessNext(t.Context()); err != nil {
				t.Fatal(err)
			}
			report, err := svc.Report(t.Context(), "refresh")
			if err != nil || report.Counts["succeeded"] != 2 {
				t.Fatalf("refresh: %+v %v", report, err)
			}
		})
	}
}

func conditionalDownload(
	t *testing.T,
	data []byte,
	keep bool,
) func(context.Context, covers.Validators) (covers.Download, error) {
	t.Helper()
	return func(_ context.Context, v covers.Validators) (covers.Download, error) {
		if keep {
			if v.ETag != "saved" {
				t.Fatal("matching local file did not send validator")
			}
			return covers.Download{NotModified: true}, nil
		}
		if v.ETag != "" || v.LastModified != "" {
			t.Fatal("missing local file sent validator")
		}
		return covers.Download{Data: data}, nil
	}
}

type fetchedSource struct{ t *testing.T }

func (s fetchedSource) FetchMain(context.Context, string, string, string) (string, error) {
	s.t.Error("already fetched source response was requested again")
	return "", fmt.Errorf("unexpected refetch")
}
func TestKnownMissingMainUsesBackupWithoutRepeatingMetadata(t *testing.T) {
	_, repo, dl, req, _, stage := deliveryFixture(t)
	req.URL = ""
	req.MainChecked = true
	req.BackupChecked = true
	req.BackupURL = "https://img.dlsite.jp/backup.webp"
	svc, err := covers.New(
		covers.Dependencies{
			Store:      repo,
			Downloader: dl,
			Source:     fetchedSource{t},
			Files:      filesystem.CoverFiles{},
			StageDir:   stage,
			Acquire:    admission.NewGate(nil).BeginManual,
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
	if err != nil || report.Counts["succeeded"] != 2 || dl.calls != 1 {
		t.Fatalf("main fallback: %+v %v", report, err)
	}
}
