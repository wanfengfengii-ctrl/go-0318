package trial

import "testing"

func TestCanAdvanceContinuousPrefix(t *testing.T) {
	if !CanAdvance(StagePrecheck, StageFillVent) {
		t.Fatal("expected precheck -> fill_vent to be allowed")
	}
	if !CanAdvance(StageReview, StageAdmission) {
		t.Fatal("expected review -> admission to be allowed")
	}
}

func TestCanAdvanceRejectsSkip(t *testing.T) {
	if CanAdvance(StagePrecheck, StageStepRamp) {
		t.Fatal("expected skipping fill_vent to be rejected")
	}
}

func TestCanAdvanceRejectsBackwards(t *testing.T) {
	if CanAdvance(StageHold, StageStepRamp) {
		t.Fatal("expected backwards advancement to be rejected")
	}
}

func TestCanAdvanceRejectsUnknown(t *testing.T) {
	if CanAdvance("bogus", StagePrecheck) {
		t.Fatal("expected unknown stage to be rejected")
	}
}

func TestStageIndex(t *testing.T) {
	if StageIndex(StagePrecheck) != 0 {
		t.Fatal("expected precheck at index 0")
	}
	if StageIndex(StageAdmission) != len(stageOrder)-1 {
		t.Fatal("expected admission at last index")
	}
	if StageIndex("nope") != -1 {
		t.Fatal("expected unknown stage index -1")
	}
}
