package configuration

import (
	"strings"
	"testing"
)

func validInput() Input {
	return Input{
		Chambers: []ChamberSection{
			{ID: "c-main", Name: "主承压舱段"},
			{ID: "c-end", Name: "端盖舱段"},
		},
		Ports: []Port{
			{ID: "p-inlet", Chamber: "c-main", Kind: PortPressureInlet},
			{ID: "p-sensor", Chamber: "c-main", Kind: PortPressureSensor, Channel: "ch-1"},
			{ID: "p-temp", Chamber: "c-end", Kind: PortTemperatureSensor, Channel: "ch-2"},
		},
		Pipes: []Pipe{
			{ID: "pipe-1", From: "p-sensor", To: "p-temp"},
		},
		SealBoundaries: []SealBoundary{
			{ID: "s-1", Chamber: "c-main", Checks: []string{"外观检查", "密封复查"}},
		},
		Steps: []PressureStep{
			{Index: 1, TargetPa: 5_000_000, RampUpPaPerS: 100_000, RampDownPaPerS: 100_000, HoldMs: 600_000, LeakLimitULPerS: 10, MaxDropPa: 50_000},
			{Index: 2, TargetPa: 10_000_000, RampUpPaPerS: 100_000, RampDownPaPerS: 100_000, HoldMs: 600_000, LeakLimitULPerS: 10, MaxDropPa: 50_000},
		},
		Calibrations: []Calibration{
			{Channel: "ch-1", Serial: "SN-P", ExpiresAtMs: 2_000_000_000_000, Summary: "压力"},
			{Channel: "ch-2", Serial: "SN-T", ExpiresAtMs: 2_000_000_000_000, Summary: "温度"},
		},
		Compensation: CompensationParams{RefTempMC: 20_000, TempCoeffPPM: 10},
	}
}

func TestFreezeValidTwoChamber(t *testing.T) {
	snap, err := Freeze(validInput())
	if err != nil {
		t.Fatalf("Freeze returned error: %v", err)
	}
	if snap.Digest == "" {
		t.Fatal("expected non-empty digest")
	}
	if snap.MaxPressurePa != 10_000_000 {
		t.Fatalf("MaxPressurePa = %d, want 10000000", snap.MaxPressurePa)
	}
}

func TestFreezeDigestStableAcrossOrdering(t *testing.T) {
	in := validInput()
	snap1, err := Freeze(in)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	// Reverse chamber and port order; the digest must be unchanged.
	in.Chambers[0], in.Chambers[1] = in.Chambers[1], in.Chambers[0]
	in.Ports[0], in.Ports[2] = in.Ports[2], in.Ports[0]
	snap2, err := Freeze(in)
	if err != nil {
		t.Fatalf("Freeze reordered: %v", err)
	}
	if snap1.Digest != snap2.Digest {
		t.Fatalf("digest not order-independent: %q != %q", snap1.Digest, snap2.Digest)
	}
}

func TestFreezeRejectsIsolatedChamber(t *testing.T) {
	in := validInput()
	in.Chambers = append(in.Chambers, ChamberSection{ID: "c-orphan", Name: "孤立舱段"})
	_, err := Freeze(in)
	assertReason(t, err, "isolated chamber")
}

func TestFreezeRejectsMissingPressureInlet(t *testing.T) {
	in := validInput()
	in.Ports = in.Ports[1:] // drop the inlet
	_, err := Freeze(in)
	assertReason(t, err, "pressure inlet")
}

func TestFreezeRejectsDuplicateInstallPosition(t *testing.T) {
	in := validInput()
	in.Ports = append(in.Ports, Port{ID: "p-sensor", Chamber: "c-main", Kind: PortPressureSensor, Channel: "ch-1"})
	_, err := Freeze(in)
	assertReason(t, err, "duplicate install position")
}

func TestFreezeRejectsUncalibratedSensor(t *testing.T) {
	in := validInput()
	in.Ports = append(in.Ports, Port{ID: "p-sensor2", Chamber: "c-end", Kind: PortPressureSensor, Channel: "ch-9"})
	_, err := Freeze(in)
	assertReason(t, err, "uncalibrated")
}

func TestFreezeRejectsDisconnectedPiping(t *testing.T) {
	in := validInput()
	in.Pipes = nil // removing the shared pipe isolates the end-cap chamber
	_, err := Freeze(in)
	assertReason(t, err, "broken pressure path")
}

func TestFreezeRejectsDuplicatePipeConnection(t *testing.T) {
	in := validInput()
	in.Pipes = append(in.Pipes, Pipe{ID: "pipe-2", From: "p-temp", To: "p-sensor"})
	_, err := Freeze(in)
	assertReason(t, err, "duplicate pipe connection")
}

func assertReason(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}
