package cleanup

import (
	"context"
	"time"
)

func (s *Service) Preview(ctx context.Context, categories []Category) (Preview, error) {
	categories, err := Normalize(categories)
	if err != nil {
		return Preview{}, err
	}
	now := time.Now().UTC()
	result := Preview{
		CheckedAt:  now,
		Categories: categories,
		Candidates: map[Category]int64{},
		Rules:      map[Category]Rule{},
	}
	for _, category := range categories {
		ids, err := s.store.CleanupCandidates(ctx, category, s.opts.Cutoff(category, now))
		if err != nil {
			return Preview{}, err
		}
		result.Candidates[category] = int64(len(ids))
		rule := Rule{Protection: "Any delivery reference protects its download; only done downloads are eligible."}
		if category == ScanSessions {
			rule = Rule{
				RetentionDays: s.opts.ScanRetention.Hours() / 24,
				Protection:    "Only terminal scans; missing finished_at falls back to started_at.",
			}
		}
		if category == PlanGenerations {
			rule = Rule{
				RetentionDays: s.opts.GenerationRetention.Hours() / 24,
				Protection:    "Only finished planning sessions; current plans and results are retained.",
			}
		}
		result.Rules[category] = rule
	}
	return result, nil
}
func (s *Service) Read(ctx context.Context) (*Run, error) { return s.store.ReadCleanupRun(ctx) }
func (s *Service) Recover(ctx context.Context) error      { return s.store.InterruptCleanupRun(ctx) }
