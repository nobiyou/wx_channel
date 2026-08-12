package poc

import "sort"

type OutcomeInput struct {
	SearchComplete        bool
	SourceExhausted       bool
	ValidWorks            int
	CompletedWorks        int
	TopLevelComments      int
	Replies               int
	RequiredFieldStatuses map[string]FieldStatus
	HumanTimedOut         bool
	HumanCancelled        bool
	SafetyFailed          bool
	CleanupFailed         bool
	SchemaFailed          bool
}

func EvaluateOutcome(input OutcomeInput) (JobStatus, CapabilityStatus, CoverageStatus, []string) {
	reasons := make([]string, 0)
	coverage := CoverageIncomplete
	if input.ValidWorks >= 10 {
		coverage = CoverageTargetMet
	} else if input.SearchComplete && input.SourceExhausted {
		coverage = CoverageSourceExhausted
	}

	if input.SafetyFailed || input.CleanupFailed {
		if input.SafetyFailed {
			reasons = append(reasons, "safety_failed")
		}
		if input.CleanupFailed {
			reasons = append(reasons, "cleanup_failed")
		}
		return JobFailed, CapabilityFailed, coverage, reasons
	}

	progress := input.ValidWorks > 0 || input.CompletedWorks > 0 || input.TopLevelComments > 0 || input.Replies > 0
	complete := input.SearchComplete && input.CompletedWorks >= input.ValidWorks && !input.SchemaFailed

	job := JobCompleted
	switch {
	case input.HumanTimedOut || input.HumanCancelled:
		if progress {
			job = JobPartial
		} else {
			job = JobRequiresHuman
		}
	case !complete:
		if progress {
			job = JobPartial
		} else {
			job = JobRequiresHuman
		}
	}

	if input.HumanTimedOut {
		reasons = append(reasons, "human_timed_out")
	}
	if input.HumanCancelled {
		reasons = append(reasons, "human_cancelled")
	}
	if input.SchemaFailed {
		reasons = append(reasons, "schema_failed")
	}

	capability := CapabilityInconclusive
	if complete && !input.HumanTimedOut && !input.HumanCancelled && input.TopLevelComments > 0 && input.Replies > 0 {
		capability = CapabilityVerified
		fieldNames := make([]string, 0, len(input.RequiredFieldStatuses))
		for name := range input.RequiredFieldStatuses {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)
		for _, name := range fieldNames {
			switch input.RequiredFieldStatuses[name] {
			case FieldPresent, FieldNotApplicable:
			case FieldMissingInSource:
				if capability == CapabilityVerified {
					capability = CapabilityVerifiedWithGaps
				}
				reasons = append(reasons, "required_field_missing:"+name)
			default:
				capability = CapabilityInconclusive
				reasons = append(reasons, "required_field_unverified:"+name)
			}
		}
	} else if input.Replies == 0 {
		reasons = append(reasons, "no_replies_observed")
	}

	return job, capability, coverage, reasons
}
