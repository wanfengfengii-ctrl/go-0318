package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"abyssal-pressure-housing-qualification/qualification"
	"abyssal-pressure-housing-qualification/store"
	"abyssal-pressure-housing-qualification/trial"
)

// completeStepsAndStages completes every pressure step, advances through all
// stages to review, and vents pressure to zero, leaving only reviews and retest
// clearance before admission.
func completeStepsAndStages(t *testing.T, svc *Service, tr *trial.Trial) {
	t.Helper()
	submitStableSamples(t, svc, tr.ID, 2000, 5)
	if _, err := svc.CompleteStep(context.Background(), CompleteStepRequest{TrialID: tr.ID, StepIndex: 1, StartMs: 2000, EndMs: 2006}); err != nil {
		t.Fatalf("complete step 1: %v", err)
	}
	submitStableSamples(t, svc, tr.ID, 3000, 5)
	if _, err := svc.CompleteStep(context.Background(), CompleteStepRequest{TrialID: tr.ID, StepIndex: 2, StartMs: 3000, EndMs: 3006}); err != nil {
		t.Fatalf("complete step 2: %v", err)
	}
	for _, st := range []string{"fill_vent", "step_ramp", "hold", "controlled_vent", "repressurize", "visual_check", "seal_check", "review"} {
		if _, err := svc.AdvanceStage(context.Background(), StageRequest{TrialID: tr.ID, Stage: st}); err != nil {
			t.Fatalf("advance to %s: %v", st, err)
		}
	}
	if err := svc.SubmitSample(context.Background(), SubmitSampleRequest{
		TrialID: tr.ID, LogicalMs: 4000, PressurePa: 0, TempMC: 20_000,
	}); err != nil {
		t.Fatalf("vent sample: %v", err)
	}
}

func submitReview(t *testing.T, svc *Service, tr *trial.Trial, operator string, expiresAt int64) {
	t.Helper()
	if err := svc.SubmitReview(context.Background(), SubmitReviewRequest{
		TrialID: tr.ID, Operator: operator, Qualification: "高级检验员", QualExpiresAt: expiresAt,
	}); err != nil {
		t.Fatalf("submit review %s: %v", operator, err)
	}
}

func TestAdmissionIssuesCredential(t *testing.T) {
	svc := newTestService(t)
	snap, tr := startTrial(t, svc)
	completeStepsAndStages(t, svc, tr)
	submitReview(t, svc, tr, "alice", 2_000_000_000_000)
	submitReview(t, svc, tr, "bob", 2_000_000_000_000)

	cred, err := svc.Finalize(context.Background(), FinalizeRequest{TrialID: tr.ID})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if cred.Digest == "" {
		t.Fatal("expected non-empty credential digest")
	}
	root, err := svc.computeEvidenceRoot(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("evidence root: %v", err)
	}
	if cred.Digest != qualification.DeriveCredential(snap.Digest, tr.ID, root) {
		t.Fatal("credential not derived from config digest, trial, and evidence root")
	}
}

func TestSameReviewerBlocksAdmission(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)
	completeStepsAndStages(t, svc, tr)
	// Only one distinct reviewer.
	submitReview(t, svc, tr, "alice", 2_000_000_000_000)
	if _, err := svc.Finalize(context.Background(), FinalizeRequest{TrialID: tr.ID}); err == nil {
		t.Fatal("expected admission to be blocked with a single reviewer")
	}
}

func TestExpiredQualificationBlocksAdmission(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)
	completeStepsAndStages(t, svc, tr)
	submitReview(t, svc, tr, "alice", 2_000_000_000_000)
	submitReview(t, svc, tr, "bob", 500) // already expired at the injected clock
	if _, err := svc.Finalize(context.Background(), FinalizeRequest{TrialID: tr.ID}); err == nil {
		t.Fatal("expected admission to be blocked by an expired qualification")
	}
}

func TestUnclearedRetestBlocksAdmission(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)
	completeStepsAndStages(t, svc, tr)
	submitReview(t, svc, tr, "alice", 2_000_000_000_000)
	submitReview(t, svc, tr, "bob", 2_000_000_000_000)
	if _, err := svc.ReportAnomaly(context.Background(), ReportAnomalyRequest{
		TrialID: tr.ID, Kind: "overpressure", PortID: "p-sensor",
	}); err != nil {
		t.Fatalf("report anomaly: %v", err)
	}
	if _, err := svc.Finalize(context.Background(), FinalizeRequest{TrialID: tr.ID}); err == nil {
		t.Fatal("expected admission to be blocked by an uncleared retest scope")
	}
}

func TestConcurrentFinalizeSingleWinner(t *testing.T) {
	svc := newTestService(t)
	_, tr := startTrial(t, svc)
	completeStepsAndStages(t, svc, tr)
	submitReview(t, svc, tr, "alice", 2_000_000_000_000)
	submitReview(t, svc, tr, "bob", 2_000_000_000_000)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, errs[0] = svc.Finalize(context.Background(), FinalizeRequest{TrialID: tr.ID})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, errs[1] = svc.Terminate(context.Background(), TerminateRequest{TrialID: tr.ID})
	}()
	close(start)
	wg.Wait()

	winners := 0
	for _, e := range errs {
		if e == nil {
			winners++
		} else if !errors.Is(e, qualification.ErrFinalStateConflict) {
			t.Fatalf("unexpected error: %v", e)
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one terminal winner, got %d (errs=%v)", winners, errs)
	}
}

func TestRecoveryPreservesCredentialAndEvidenceRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	svc := New(st)
	svc.SetClock(func() int64 { return testNow })
	snap, tr := startTrial(t, svc)
	completeStepsAndStages(t, svc, tr)
	submitReview(t, svc, tr, "alice", 2_000_000_000_000)
	submitReview(t, svc, tr, "bob", 2_000_000_000_000)
	cred, err := svc.Finalize(context.Background(), FinalizeRequest{TrialID: tr.ID})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st2.Close()
	svc2 := New(st2)

	tr2, err := svc2.GetTrial(context.Background(), tr.ID)
	if err != nil {
		t.Fatalf("get trial after reopen: %v", err)
	}
	if tr2.Terminal != trial.TerminalAdmitted {
		t.Fatalf("terminal = %s, want admitted", tr2.Terminal)
	}
	cred2, err := svc2.GetCredential(context.Background(), tr.ID)
	if err != nil {
		t.Fatalf("get credential after reopen: %v", err)
	}
	if cred2.Digest != cred.Digest {
		t.Fatalf("credential changed across restart: %s != %s", cred2.Digest, cred.Digest)
	}
	root, err := svc2.computeEvidenceRoot(context.Background(), tr.ID, 1)
	if err != nil {
		t.Fatalf("evidence root after reopen: %v", err)
	}
	if cred2.Digest != qualification.DeriveCredential(snap.Digest, tr.ID, root) {
		t.Fatal("credential does not match re-derived evidence root after restart")
	}
}
