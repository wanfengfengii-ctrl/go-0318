package configuration

import "sort"

// ChamberByID returns the chamber section with the given id, or nil.
func (s *Snapshot) ChamberByID(id string) *ChamberSection {
	for i := range s.Chambers {
		if s.Chambers[i].ID == id {
			return &s.Chambers[i]
		}
	}
	return nil
}

// PortByID returns the install position with the given id, or nil.
func (s *Snapshot) PortByID(id string) *Port {
	for i := range s.Ports {
		if s.Ports[i].ID == id {
			return &s.Ports[i]
		}
	}
	return nil
}

// StepByIndex returns the pressure step with the given one-based index, or nil.
func (s *Snapshot) StepByIndex(index int) *PressureStep {
	for i := range s.Steps {
		if s.Steps[i].Index == index {
			return &s.Steps[i]
		}
	}
	return nil
}

// SealBoundaryByID returns the seal boundary with the given id, or nil.
func (s *Snapshot) SealBoundaryByID(id string) *SealBoundary {
	for i := range s.SealBoundaries {
		if s.SealBoundaries[i].ID == id {
			return &s.SealBoundaries[i]
		}
	}
	return nil
}

// PressureSensorPorts returns the ids of every pressure sensor port, sorted.
func (s *Snapshot) PressureSensorPorts() []string {
	var out []string
	for _, p := range s.Ports {
		if p.Kind == PortPressureSensor {
			out = append(out, p.ID)
		}
	}
	sort.Strings(out)
	return out
}

// InletPort returns the id of the pressure inlet port, or "" when absent.
func (s *Snapshot) InletPort() string {
	for _, p := range s.Ports {
		if p.Kind == PortPressureInlet {
			return p.ID
		}
	}
	return ""
}

// PortsInChamber returns the ids of every port installed on the chamber,
// sorted.
func (s *Snapshot) PortsInChamber(chamber string) []string {
	var out []string
	for _, p := range s.Ports {
		if p.Chamber == chamber {
			out = append(out, p.ID)
		}
	}
	sort.Strings(out)
	return out
}

// SharedPipesFor returns the ids of every pipe touching one of the given
// ports, sorted, so anomaly propagation can follow shared piping.
func (s *Snapshot) SharedPipesFor(ports ...string) []string {
	inSet := map[string]bool{}
	for _, p := range ports {
		inSet[p] = true
	}
	var out []string
	for _, p := range s.Pipes {
		if inSet[p.From] || inSet[p.To] {
			out = append(out, p.ID)
		}
	}
	sort.Strings(out)
	return out
}

// ConnectedChambers returns the set of chamber ids reachable from the given
// chamber through shared pipes, including the chamber itself.
func (s *Snapshot) ConnectedChambers(chamber string) []string {
	adj := map[string]map[string]bool{}
	add := func(a, b string) {
		if adj[a] == nil {
			adj[a] = map[string]bool{}
		}
		adj[a][b] = true
	}
	for _, p := range s.Pipes {
		from := s.portChamber(p.From)
		to := s.portChamber(p.To)
		if from != "" && to != "" {
			add(from, to)
			add(to, from)
		}
	}
	if adj[chamber] == nil {
		adj[chamber] = map[string]bool{}
	}
	seen := map[string]bool{chamber: true}
	queue := []string{chamber}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for n := range adj[cur] {
			if !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *Snapshot) portChamber(portID string) string {
	p := s.PortByID(portID)
	if p == nil {
		return ""
	}
	return p.Chamber
}
