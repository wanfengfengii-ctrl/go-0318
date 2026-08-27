package qualification

import (
	"fmt"
	"sort"

	"abyssal-pressure-housing-qualification/configuration"
)

// Anomaly locates an isolation-triggering event on the frozen configuration.
// Exactly one of PortID, ChamberID, or SealID should identify the origin.
type Anomaly struct {
	Kind      AnomalyKind
	PortID    string
	ChamberID string
	SealID    string
}

// PropagateRetest derives the retest scope for a single anomaly by walking the
// frozen configuration: overpressure and pressure drop propagate to the owning
// chamber, seal leaks propagate to the seal boundary, and valve mismatches
// propagate across shared piping to every connected chamber.
func PropagateRetest(snap *configuration.Snapshot, a Anomaly) (RetestSet, error) {
	rs := RetestSet{}
	switch a.Kind {
	case AnomalyOverpressure, AnomalyPressureDrop:
		chamber, err := resolveChamber(snap, a)
		if err != nil {
			return rs, err
		}
		rs.Members = append(rs.Members, chamberPortMembers(snap, chamber, "压力检查")...)
	case AnomalySealLeak:
		sealID := a.SealID
		if sealID == "" {
			// Derive the seal boundary from the origin chamber when not explicit.
			sealID = firstSealForChamber(snap, a.ChamberID)
		}
		seal := snap.SealBoundaryByID(sealID)
		if seal == nil {
			return rs, fmt.Errorf("seal leak references unknown seal boundary %q", sealID)
		}
		for _, check := range seal.Checks {
			rs.Members = append(rs.Members, RetestMember{Chamber: seal.Chamber, CheckType: check})
		}
	case AnomalyValveMismatch:
		chamber, err := resolveChamber(snap, a)
		if err != nil {
			return rs, err
		}
		for _, ch := range snap.ConnectedChambers(chamber) {
			rs.Members = append(rs.Members, chamberPortMembers(snap, ch, "阀位检查")...)
		}
	default:
		return rs, fmt.Errorf("unknown anomaly kind %q", a.Kind)
	}
	return dedupeRetest(rs), nil
}

// MergeRetest merges multiple anomaly retest sets into one canonically ordered,
// de-duplicated set.
func MergeRetest(sets ...RetestSet) RetestSet {
	var out RetestSet
	for _, s := range sets {
		out.Members = append(out.Members, s.Members...)
	}
	return dedupeRetest(out)
}

func resolveChamber(snap *configuration.Snapshot, a Anomaly) (string, error) {
	if a.ChamberID != "" {
		return a.ChamberID, nil
	}
	if a.PortID != "" {
		p := snap.PortByID(a.PortID)
		if p == nil {
			return "", fmt.Errorf("anomaly references unknown port %q", a.PortID)
		}
		return p.Chamber, nil
	}
	return "", fmt.Errorf("anomaly has no resolvable location")
}

func chamberPortMembers(snap *configuration.Snapshot, chamber, checkType string) []RetestMember {
	ports := snap.PortsInChamber(chamber)
	members := make([]RetestMember, 0, len(ports))
	for _, pid := range ports {
		members = append(members, RetestMember{Chamber: chamber, PortID: pid, CheckType: checkType})
	}
	return members
}

func firstSealForChamber(snap *configuration.Snapshot, chamber string) string {
	for _, s := range snap.SealBoundaries {
		if s.Chamber == chamber {
			return s.ID
		}
	}
	return ""
}

func dedupeRetest(rs RetestSet) RetestSet {
	sort.Slice(rs.Members, func(i, j int) bool {
		a, b := rs.Members[i], rs.Members[j]
		if a.Chamber != b.Chamber {
			return a.Chamber < b.Chamber
		}
		if a.PortID != b.PortID {
			return a.PortID < b.PortID
		}
		return a.CheckType < b.CheckType
	})
	out := rs.Members[:0]
	for i, m := range rs.Members {
		if i > 0 && rs.Members[i-1] == m {
			continue
		}
		out = append(out, m)
	}
	rs.Members = out
	return rs
}
