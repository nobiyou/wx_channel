package poc

import "testing"

func TestTimeoutBeforeWorksRequiresHuman(t *testing.T) {
	job, capability, coverage, reasons := EvaluateOutcome(OutcomeInput{HumanTimedOut: true})
	if job != JobRequiresHuman || capability != CapabilityInconclusive || coverage != CoverageIncomplete || !containsReason(reasons, "human_timed_out") {
		t.Fatalf("job=%s capability=%s coverage=%s reasons=%v", job, capability, coverage, reasons)
	}
}

func TestTimeoutAfterWorkIsPartial(t *testing.T) {
	job, capability, _, _ := EvaluateOutcome(OutcomeInput{ValidWorks: 1, CompletedWorks: 1, TopLevelComments: 2, HumanTimedOut: true})
	if job != JobPartial || capability != CapabilityInconclusive {
		t.Fatalf("job=%s capability=%s", job, capability)
	}
}

func TestThreeDimensionalStatusForThreeExhaustedWorks(t *testing.T) {
	job, capability, coverage, reasons := EvaluateOutcome(OutcomeInput{
		SearchComplete: true, SourceExhausted: true, ValidWorks: 3, CompletedWorks: 3,
		TopLevelComments: 4, Replies: 2,
		RequiredFieldStatuses: map[string]FieldStatus{"comment_id": FieldPresent, "ip_location": FieldMissingInSource},
	})
	if job != JobCompleted || capability != CapabilityVerifiedWithGaps || coverage != CoverageSourceExhausted {
		t.Fatalf("job=%s capability=%s coverage=%s reasons=%v", job, capability, coverage, reasons)
	}
}

func TestNoRepliesIsCapabilityInconclusiveNotCoverageFailure(t *testing.T) {
	job, capability, coverage, reasons := EvaluateOutcome(OutcomeInput{
		SearchComplete: true, SourceExhausted: true, ValidWorks: 3, CompletedWorks: 3, TopLevelComments: 4,
	})
	if job != JobCompleted || capability != CapabilityInconclusive || coverage != CoverageSourceExhausted || !containsReason(reasons, "no_replies_observed") {
		t.Fatalf("job=%s capability=%s coverage=%s reasons=%v", job, capability, coverage, reasons)
	}
}

func TestSafetyOrCleanupFailureOverridesCapability(t *testing.T) {
	job, capability, _, _ := EvaluateOutcome(OutcomeInput{SearchComplete: true, ValidWorks: 10, CompletedWorks: 10, TopLevelComments: 1, Replies: 1, CleanupFailed: true})
	if job != JobFailed || capability != CapabilityFailed {
		t.Fatalf("job=%s capability=%s", job, capability)
	}
}

func TestSafetyRedactionLeavesCapabilityInconclusive(t *testing.T) {
	_, capability, _, reasons := EvaluateOutcome(OutcomeInput{
		SearchComplete: true, ValidWorks: 10, CompletedWorks: 10, TopLevelComments: 1, Replies: 1,
		RequiredFieldStatuses: map[string]FieldStatus{"content": FieldRedactedForSafety},
	})
	if capability != CapabilityInconclusive || !containsReason(reasons, "required_field_unverified:content") {
		t.Fatalf("capability=%s reasons=%v", capability, reasons)
	}
}
