package works

// ImportRelationReport saves an external subject's relation snapshot using the
// same validation and replacement rules as a catalog work's metadata import.
// Retrieval finishes before this entry opens its storage transaction.
func (s *Service) ImportRelationReport(target WorkCode, req ImportRelations) (ImportOutcome, *Error) {
	group, invalid := validateRelationsPayload(target, req)
	if invalid != nil {
		return ImportOutcome{}, invalid
	}
	outcome, err := s.store.ImportRelationReport(target, group)
	if err != nil {
		return ImportOutcome{}, asInternal(err)
	}
	return outcome, nil
}
