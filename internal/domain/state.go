package domain

func CanTransition(from, to JobStatus) bool {
	allowed := map[JobStatus]map[JobStatus]bool{
		StatusDraft:           {StatusReady: true},
		StatusReady:           {StatusCapturing: true},
		StatusCapturing:       {StatusRemediation: true, StatusPendingApproval: true},
		StatusRemediation:     {StatusPendingApproval: true, StatusCapturing: true},
		StatusPendingApproval: {StatusRemediation: true, StatusFrozen: true},
		StatusFrozen:          {StatusCertified: true},
	}
	return allowed[from][to]
}

func Transition(job *DigitizationJob, to JobStatus) error {
	if job.Status == StatusCertified {
		return ErrFrozen
	}
	if job.Status == StatusFrozen && to != StatusCertified {
		return ErrFrozen
	}
	if !CanTransition(job.Status, to) {
		return ErrInvalidTransition
	}
	job.Status = to
	return nil
}

func Mutable(job DigitizationJob) error {
	if job.Status == StatusFrozen || job.Status == StatusCertified {
		return ErrFrozen
	}
	return nil
}
