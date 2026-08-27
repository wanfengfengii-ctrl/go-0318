package evidence

import "fmt"

// StepParams are the frozen judgement parameters for one pressure step,
// self-contained so the evidence engine need not depend on the configuration
// package.
type StepParams struct {
	TargetPa        int64
	RampUpPaPerS    int64
	RampDownPaPerS  int64
	HoldMs          int64
	LeakLimitULPerS int64
	MaxDropPa       int64
	VolumeUL        int64
	MaxPressurePa   int64
}

// EvidenceWindow is a frozen half-open sampling window [StartMs, EndMs) with
// its strictly ordered samples. A window is qualified when every computed
// metric stays within the step thresholds.
type EvidenceWindow struct {
	TrialID   string   `json:"trial_id"`
	Round     int      `json:"round"`
	StepIndex int      `json:"step_index"`
	StartMs   int64    `json:"start_ms"`
	EndMs     int64    `json:"end_ms"`
	Samples   []Sample `json:"samples"`
}

// Validate verifies that the window is non-empty, that its samples are strictly
// ordered by logical clock, and that every sample falls inside the half-open
// window boundary.
func (w EvidenceWindow) Validate() error {
	if len(w.Samples) == 0 {
		return fmt.Errorf("%w: empty window", ErrWindowMismatch)
	}
	if w.EndMs <= w.StartMs {
		return fmt.Errorf("%w: end %d not after start %d", ErrWindowMismatch, w.EndMs, w.StartMs)
	}
	var prev int64
	for i, s := range w.Samples {
		if !InWindow(s.LogicalMs, w.StartMs, w.EndMs) {
			return fmt.Errorf("%w: sample %d at %d outside [%d,%d)", ErrWindowMismatch, i, s.LogicalMs, w.StartMs, w.EndMs)
		}
		if i > 0 {
			if err := ValidateSampleOrder(prev, s.LogicalMs); err != nil {
				return err
			}
		}
		prev = s.LogicalMs
	}
	return nil
}

// Evaluate computes the evidence metrics for a window and enforces every
// threshold: compensated pressure below the chamber limit, ramp rate below the
// ramp limit, pressure drop below the drop limit, and leak rate below the leak
// limit. Any overflow or division by zero aborts the whole evaluation.
func Evaluate(w EvidenceWindow, p StepParams, comp CompensationParams) (Metrics, error) {
	if err := w.Validate(); err != nil {
		return Metrics{}, err
	}

	compensated := make([]int64, len(w.Samples))
	var peak int64
	var peakIdx int
	for i, s := range w.Samples {
		cp, err := CompensatedPressure(s.PressurePa, s.TempMC, comp)
		if err != nil {
			return Metrics{}, err
		}
		if cp > p.MaxPressurePa {
			return Metrics{}, fmt.Errorf("%w: compensated %d exceeds limit %d", ErrOverpressure, cp, p.MaxPressurePa)
		}
		compensated[i] = cp
		if cp > peak {
			peak = cp
			peakIdx = i
		}
	}

	// Ramp rate is the largest absolute rate between consecutive samples.
	var maxRate int64
	for i := 1; i < len(w.Samples); i++ {
		rate, err := RampRate(compensated[i-1], compensated[i], w.Samples[i-1].LogicalMs, w.Samples[i].LogicalMs)
		if err != nil {
			return Metrics{}, err
		}
		arate, err := Abs(rate)
		if err != nil {
			return Metrics{}, err
		}
		if arate > maxRate {
			maxRate = arate
		}
	}
	if maxRate > p.RampUpPaPerS {
		return Metrics{}, fmt.Errorf("%w: ramp %d exceeds limit %d", ErrRateExceeded, maxRate, p.RampUpPaPerS)
	}

	// Pressure drop is the fall from the peak compensated pressure to the final
	// sample. A rising tail yields zero drop.
	drop, err := Sub(compensated[peakIdx], compensated[len(compensated)-1])
	if err != nil {
		return Metrics{}, err
	}
	drop, err = Abs(drop)
	if err != nil {
		return Metrics{}, err
	}
	if drop > p.MaxDropPa {
		return Metrics{}, fmt.Errorf("%w: drop %d exceeds limit %d", ErrDropExceeded, drop, p.MaxDropPa)
	}

	// Leak rate is estimated from the pressure decay over the window span using
	// Boyle's law, parameterised by the chamber volume.
	first := w.Samples[0].LogicalMs
	last := w.Samples[len(w.Samples)-1].LogicalMs
	elapsed, err := Sub(last, first)
	if err != nil {
		return Metrics{}, err
	}
	leak, err := LeakRate(peak, compensated[len(compensated)-1], elapsed, p.VolumeUL)
	if err != nil {
		return Metrics{}, err
	}
	if leak > p.LeakLimitULPerS {
		return Metrics{}, fmt.Errorf("%w: leak %d exceeds limit %d", ErrLeakLimit, leak, p.LeakLimitULPerS)
	}

	return Metrics{
		CompensatedPressurePa: compensated[len(compensated)-1],
		RampRatePaPerS:        maxRate,
		PressureDropPa:        drop,
		LeakRateULPerS:        leak,
	}, nil
}
