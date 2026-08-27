// Package configuration implements the trial configuration catalog: the
// frozen chamber structure, install positions (ports), pressure steps,
// calibration summaries, and integer judgement parameters. It validates seal
// boundaries and pressure-path connectivity and produces an immutable,
// canonically ordered snapshot with a stable digest.
package configuration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PortKind classifies an install position / connection point.
type PortKind string

const (
	PortPressureInlet     PortKind = "pressure_inlet"
	PortPressureSensor    PortKind = "pressure_sensor"
	PortTemperatureSensor PortKind = "temperature_sensor"
	PortValve             PortKind = "valve"
	PortPump              PortKind = "pump"
)

// ChamberSection is a closed pressure-bearing chamber section.
type ChamberSection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	VolumeUL int64  `json:"volume_ul"` // internal volume in microlitres, for leak estimation
}

// Port is an install position on a chamber section.
type Port struct {
	ID      string   `json:"id"`
	Chamber string   `json:"chamber"`
	Kind    PortKind `json:"kind"`
	Channel string   `json:"channel"` // calibrated channel reference, empty when not calibrated
}

// Pipe is a shared pipeline connecting two install positions, potentially
// across chamber sections. Shared piping is used to propagate anomaly retest
// scope between chambers.
type Pipe struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// SealBoundary is a seal between components that requires an inspection check.
type SealBoundary struct {
	ID      string   `json:"id"`
	Chamber string   `json:"chamber"`
	Checks  []string `json:"checks"`
}

// PressureStep is one step of the graded pressurisation ladder.
type PressureStep struct {
	Index           int   `json:"index"`
	TargetPa        int64 `json:"target_pa"`
	RampUpPaPerS    int64 `json:"ramp_up_pa_per_s"`
	RampDownPaPerS  int64 `json:"ramp_down_pa_per_s"`
	HoldMs          int64 `json:"hold_ms"`
	LeakLimitULPerS int64 `json:"leak_limit_ul_per_s"`
	MaxDropPa       int64 `json:"max_drop_pa"`
}

// Calibration is a frozen sensor calibration summary.
type Calibration struct {
	Channel     string `json:"channel"`
	Serial      string `json:"serial"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
	Summary     string `json:"summary"`
}

// CompensationParams are the frozen integer parameters for temperature
// compensated pressure: reference temperature (millicelsius) and a linear
// coefficient in parts-per-million per millicelsius.
type CompensationParams struct {
	RefTempMC    int64 `json:"ref_temp_mc"`
	TempCoeffPPM int64 `json:"temp_coeff_ppm"`
}

// Input is the raw configuration submitted before freezing.
type Input struct {
	Chambers       []ChamberSection   `json:"chambers"`
	Ports          []Port             `json:"ports"`
	Pipes          []Pipe             `json:"pipes"`
	SealBoundaries []SealBoundary     `json:"seal_boundaries"`
	Steps          []PressureStep     `json:"steps"`
	Calibrations   []Calibration      `json:"calibrations"`
	Compensation   CompensationParams `json:"compensation"`
}

// Snapshot is an immutable, canonically ordered frozen configuration.
type Snapshot struct {
	Digest         string             `json:"digest"`
	Chambers       []ChamberSection   `json:"chambers"`
	Ports          []Port             `json:"ports"`
	Pipes          []Pipe             `json:"pipes"`
	SealBoundaries []SealBoundary     `json:"seal_boundaries"`
	Steps          []PressureStep     `json:"steps"`
	Calibrations   []Calibration      `json:"calibrations"`
	Compensation   CompensationParams `json:"compensation"`
	MaxPressurePa  int64              `json:"max_pressure_pa"`
}

// ValidationError reports one or more reasons a configuration was rejected.
type ValidationError struct {
	Reasons []string
}

func (e *ValidationError) Error() string {
	return "configuration validation failed: " + strings.Join(e.Reasons, "; ")
}

// Freeze validates the submitted configuration, canonicalises it, and returns
// an immutable snapshot whose digest is stable across process restarts.
func Freeze(in Input) (*Snapshot, error) {
	var reasons []string
	add := func(format string, args ...any) { reasons = append(reasons, fmt.Sprintf(format, args...)) }

	if len(in.Chambers) == 0 {
		add("no chambers supplied")
	}

	// Duplicate chamber IDs.
	chamberIDs := map[string]bool{}
	for _, c := range in.Chambers {
		if c.ID == "" {
			add("chamber with empty id")
		}
		if chamberIDs[c.ID] {
			add("duplicate chamber id %q", c.ID)
		}
		chamberIDs[c.ID] = true
	}

	// Duplicate port IDs and port references to unknown chambers.
	portIDs := map[string]bool{}
	portChamber := map[string]string{}
	portKind := map[string]PortKind{}
	chamberHasPort := map[string]bool{}
	var inlets, sensors int
	calibrated := map[string]bool{}
	for _, c := range in.Calibrations {
		calibrated[c.Channel] = true
	}

	for _, p := range in.Ports {
		if p.ID == "" {
			add("port with empty id")
		}
		if portIDs[p.ID] {
			add("duplicate install position %q", p.ID)
		}
		portIDs[p.ID] = true
		portChamber[p.ID] = p.Chamber
		portKind[p.ID] = p.Kind
		if !chamberIDs[p.Chamber] {
			add("port %q references unknown chamber %q", p.ID, p.Chamber)
		}
		chamberHasPort[p.Chamber] = true

		switch p.Kind {
		case PortPressureInlet:
			inlets++
		case PortPressureSensor:
			sensors++
			if p.Channel == "" {
				add("pressure sensor %q has no channel", p.ID)
			} else if !calibrated[p.Channel] {
				add("pressure sensor %q on uncalibrated channel %q", p.ID, p.Channel)
			}
		}
	}

	if inlets < 1 {
		add("missing pressure inlet")
	}
	if sensors < 1 {
		add("no valid pressure sensor")
	}

	// Isolated chamber sections: every chamber must have at least one port.
	for _, c := range in.Chambers {
		if !chamberHasPort[c.ID] {
			add("isolated chamber %q with no install position", c.ID)
		}
	}

	// Seal boundaries must reference existing chambers and carry checks.
	sealIDs := map[string]bool{}
	for _, s := range in.SealBoundaries {
		if s.ID == "" {
			add("seal boundary with empty id")
		}
		if sealIDs[s.ID] {
			add("duplicate seal boundary %q", s.ID)
		}
		sealIDs[s.ID] = true
		if !chamberIDs[s.Chamber] {
			add("seal boundary %q references unknown chamber %q", s.ID, s.Chamber)
		}
		if len(s.Checks) == 0 {
			add("seal boundary %q has no inspection check", s.ID)
		}
	}

	// Pressure steps: strictly increasing positive targets and positive rates.
	seenStep := map[int]bool{}
	var prev int64
	for _, st := range in.Steps {
		if seenStep[st.Index] {
			add("duplicate pressure step index %d", st.Index)
		}
		seenStep[st.Index] = true
		if st.TargetPa <= prev {
			add("pressure step %d target not strictly increasing", st.Index)
		}
		prev = st.TargetPa
		if st.RampUpPaPerS <= 0 || st.RampDownPaPerS <= 0 {
			add("pressure step %d has non-positive ramp rate", st.Index)
		}
		if st.HoldMs <= 0 {
			add("pressure step %d has non-positive hold duration", st.Index)
		}
	}
	if len(in.Steps) == 0 {
		add("no pressure steps supplied")
	}

	// Pipes: duplicate IDs, unknown or self-referential endpoints.
	pipeIDs := map[string]bool{}
	type pipeEdge struct{ a, b string }
	pipeEdges := map[pipeEdge]bool{}
	for _, pi := range in.Pipes {
		if pi.ID == "" {
			add("pipe with empty id")
		}
		if pipeIDs[pi.ID] {
			add("duplicate pipe %q", pi.ID)
		}
		pipeIDs[pi.ID] = true
		if !portIDs[pi.From] {
			add("pipe %q references unknown port %q", pi.ID, pi.From)
		}
		if !portIDs[pi.To] {
			add("pipe %q references unknown port %q", pi.ID, pi.To)
		}
		if pi.From == pi.To && pi.From != "" {
			add("pipe %q connects a port to itself", pi.ID)
		}
		if pi.From != "" && pi.To != "" && pi.From != pi.To {
			a, b := pi.From, pi.To
			if a > b {
				a, b = b, a
			}
			if pipeEdges[pipeEdge{a, b}] {
				add("duplicate pipe connection between %q and %q", pi.From, pi.To)
			}
			pipeEdges[pipeEdge{a, b}] = true
		}
	}

	// Connectivity: every chamber must lie on a connected pressure-bearing path
	// reachable from the pressure inlet; the inlet must reach a pressure sensor.
	if len(reasons) == 0 {
		if err := validateConnectivity(in, portChamber, portKind); err != nil {
			add("%s", err.Error())
		}
	}

	if len(reasons) > 0 {
		return nil, &ValidationError{Reasons: reasons}
	}

	snap := canonicalise(in)
	snap.Digest = digest(snap)
	return snap, nil
}

// canonicalise sorts every collection so the digest is order independent.
func canonicalise(in Input) *Snapshot {
	chambers := append([]ChamberSection(nil), in.Chambers...)
	sort.Slice(chambers, func(i, j int) bool { return chambers[i].ID < chambers[j].ID })

	ports := append([]Port(nil), in.Ports...)
	sort.Slice(ports, func(i, j int) bool { return ports[i].ID < ports[j].ID })

	pipes := append([]Pipe(nil), in.Pipes...)
	sort.Slice(pipes, func(i, j int) bool { return pipes[i].ID < pipes[j].ID })

	seals := append([]SealBoundary(nil), in.SealBoundaries...)
	sort.Slice(seals, func(i, j int) bool { return seals[i].ID < seals[j].ID })
	for i := range seals {
		checks := append([]string(nil), seals[i].Checks...)
		sort.Strings(checks)
		seals[i].Checks = checks
	}

	steps := append([]PressureStep(nil), in.Steps...)
	sort.Slice(steps, func(i, j int) bool { return steps[i].Index < steps[j].Index })

	calibrations := append([]Calibration(nil), in.Calibrations...)
	sort.Slice(calibrations, func(i, j int) bool { return calibrations[i].Channel < calibrations[j].Channel })

	var max int64
	for _, s := range steps {
		if s.TargetPa > max {
			max = s.TargetPa
		}
	}

	return &Snapshot{
		Chambers:       chambers,
		Ports:          ports,
		Pipes:          pipes,
		SealBoundaries: seals,
		Steps:          steps,
		Calibrations:   calibrations,
		Compensation:   in.Compensation,
		MaxPressurePa:  max,
	}
}

// digest computes the stable SHA-256 digest of the canonical snapshot, with
// the digest field itself excluded.
func digest(s *Snapshot) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(struct {
		Chambers       []ChamberSection   `json:"chambers"`
		Ports          []Port             `json:"ports"`
		Pipes          []Pipe             `json:"pipes"`
		SealBoundaries []SealBoundary     `json:"seal_boundaries"`
		Steps          []PressureStep     `json:"steps"`
		Calibrations   []Calibration      `json:"calibrations"`
		Compensation   CompensationParams `json:"compensation"`
	}{
		s.Chambers, s.Ports, s.Pipes, s.SealBoundaries, s.Steps, s.Calibrations, s.Compensation,
	})
	return hex.EncodeToString(h.Sum(nil))
}
