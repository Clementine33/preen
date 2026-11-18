package works

// RandomPick is the work Random chose and where the ordinary listing under the
// same filter and order places it (0-based), so a caller can open its page.
type RandomPick struct {
	ID    WorkCode
	Index int
}

// Random picks one work of the filtered listing at random, preferring works
// never listened to. exclude (empty for none) is the work the caller is
// already on: it is avoided unless it is the only candidate. The bool reports
// whether the filter lists any work.
func (s *Service) Random(filter Filter, exclude WorkCode) (RandomPick, bool, *Error) {
	if err := filter.Validate(); err != nil {
		return RandomPick{}, false, err
	}
	pick, found, err := s.store.RandomWork(filter, exclude)
	if err != nil {
		return RandomPick{}, false, asInternal(err)
	}
	return pick, found, nil
}
