package workset

// SetExclusiveEnqueueGuard supplies the scan-exclusive guard for opted-in tasks.
func (s *serviceImpl) SetExclusiveEnqueueGuard(guard EnqueueGuard) {
	s.exclusiveEnqueueGuard = guard
}

func exclusiveTask(t Task) bool {
	admission, ok := t.(ExclusiveExecutionAdmission)
	return ok && admission.RequiresExclusiveExecution()
}

func (s *serviceImpl) checkGenerationAdmission(root string) error {
	var kinds []string
	for _, t := range s.tasks {
		if exclusiveTask(t) {
			kinds = append(kinds, t.Kind())
		}
	}
	active, err := s.repo.HasActiveExecutionForRootKinds(root, kinds)
	if err != nil {
		return NewError(ErrKindInternal, "INTERNAL", "failed to check exclusive executions", err)
	}
	if active {
		return NewError(ErrKindConflict, "EXECUTION_IN_PROGRESS", "wait for directory execution before generating", nil)
	}
	return nil
}

func (s *serviceImpl) enqueueExecution(op *Operation, create func() error) error {
	t, err := s.requireTask(op.OperationType)
	if err != nil {
		return err
	}
	if !exclusiveTask(t) {
		return s.enqueue(create)
	}
	w, err := s.repo.GetWorkset(op.WorksetID)
	if err != nil {
		return err
	}
	write := func() error {
		active, err := s.repo.HasActiveGenerationForRoot(w.RootPath)
		if err != nil {
			return NewError(ErrKindInternal, "INTERNAL", "failed to check library generation", err)
		}
		if active {
			return notExecutable([]string{ExecBlockedGeneration})
		}
		return create()
	}
	if s.exclusiveEnqueueGuard != nil {
		return s.exclusiveEnqueueGuard(write)
	}
	return s.enqueue(write)
}
