// Package cleanup owns deployment-wide manual record cleanup, its selected
// categories, finite runs and process-owned worker lifecycle. Storage commits
// deletions and progress together; admission yields between batches.
package cleanup

import (
	"context"
	"errors"
	"slices"
	"time"
)

type Category string

const (
	ScanSessions    Category = "scan_sessions"
	PlanGenerations Category = "plan_generations"
	CoverDownloads  Category = "cover_downloads"
)

var supportedCategories = []Category{ScanSessions, PlanGenerations, CoverDownloads}
var ErrInvalidCategories = errors.New("select at least one supported cleanup category")
var ErrNotFound = errors.New("cleanup run not found")

type Count struct {
	Checked int64 `json:"checked"`
	Deleted int64 `json:"deleted"`
	Skipped int64 `json:"skipped"`
}
type Run struct {
	ID         string             `json:"run_id"`
	Categories []Category         `json:"categories"`
	Status     string             `json:"status"`
	Phase      Category           `json:"phase"`
	StartedAt  time.Time          `json:"started_at"`
	FinishedAt *time.Time         `json:"finished_at"`
	Counts     map[Category]Count `json:"counts"`
	Reason     string             `json:"reason"`
}
type Rule struct {
	RetentionDays float64 `json:"retention_days"`
	Protection    string  `json:"protection"`
}
type Preview struct {
	CheckedAt  time.Time          `json:"checked_at"`
	Categories []Category         `json:"categories"`
	Candidates map[Category]int64 `json:"candidates"`
	Rules      map[Category]Rule  `json:"rules"`
}
type Options struct {
	ScanRetention       time.Duration
	GenerationRetention time.Duration
	BatchRows           int
	YieldDelay          time.Duration
}

func (o Options) Cutoff(category Category, now time.Time) time.Time {
	switch category {
	case ScanSessions:
		return now.Add(-o.ScanRetention)
	case PlanGenerations:
		return now.Add(-o.GenerationRetention)
	case CoverDownloads:
		return now
	default:
		return now
	}
}

type Store interface {
	CleanupCandidates(context.Context, Category, time.Time) ([]string, error)
	CreateCleanupRun(context.Context, Run) error
	ReadCleanupRun(context.Context) (*Run, error)
	CleanupBatch(context.Context, string, Category, time.Time, []string) error
	SetCleanupPhase(context.Context, string, Category) error
	StopCleanupRun(context.Context, string) (*Run, error)
	FinishCleanupRun(context.Context, string, string, string) error
	InterruptCleanupRun(context.Context) error
}
type Gate interface {
	BeginCleanup() (func(), func(), error)
	ContinueCleanup() (func(), error)
}

func Normalize(categories []Category) ([]Category, error) {
	selected := map[Category]bool{}
	for _, category := range categories {
		if !slices.Contains(supportedCategories, category) {
			return nil, ErrInvalidCategories
		}
		selected[category] = true
	}
	if len(selected) == 0 {
		return nil, ErrInvalidCategories
	}
	normalized := make([]Category, 0, len(selected))
	for _, category := range supportedCategories {
		if selected[category] {
			normalized = append(normalized, category)
		}
	}
	return normalized, nil
}
