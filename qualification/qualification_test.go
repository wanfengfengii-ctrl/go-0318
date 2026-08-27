package qualification

import "testing"

func TestDeriveCredentialDeterministic(t *testing.T) {
	a := DeriveCredential("cfg-digest", "trial-1", "evidence-root")
	b := DeriveCredential("cfg-digest", "trial-1", "evidence-root")
	if a == "" || a != b {
		t.Fatalf("credential not deterministic: %q != %q", a, b)
	}
	c := DeriveCredential("cfg-other", "trial-1", "evidence-root")
	if a == c {
		t.Fatal("different inputs produced identical credential")
	}
}

func TestValidateReviews(t *testing.T) {
	r1 := Review{Operator: "alice", Qualification: "高级检验员"}
	r2 := Review{Operator: "bob", Qualification: "高级检验员"}
	if !ValidateReviews(r1, r2) {
		t.Fatal("expected two distinct reviewers to be valid")
	}
	if ValidateReviews(r1, r1) {
		t.Fatal("expected same reviewer to be rejected")
	}
	blank := Review{Operator: "", Qualification: "高级检验员"}
	if ValidateReviews(blank, r2) {
		t.Fatal("expected blank operator to be rejected")
	}
}
