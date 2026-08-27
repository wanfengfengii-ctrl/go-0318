package configuration

import (
	"fmt"
	"sort"
)

// validateConnectivity builds a chamber-adjacency graph from the ports and
// shared pipes, then verifies that every chamber is connected to the chamber
// holding the pressure inlet and that the inlet can reach at least one pressure
// sensor. It returns a descriptive error when the pressure path is broken.
func validateConnectivity(in Input, portChamber map[string]string, portKind map[string]PortKind) error {
	// Adjacency between chambers: two chambers are adjacent when a pipe links a
	// port in one to a port in the other. Ports within one chamber are already
	// mutually connected through the chamber volume.
	adj := map[string]map[string]bool{}
	addEdge := func(a, b string) {
		if adj[a] == nil {
			adj[a] = map[string]bool{}
		}
		if adj[b] == nil {
			adj[b] = map[string]bool{}
		}
		adj[a][b] = true
		adj[b][a] = true
	}

	var inletChamber string
	var sensorChambers []string
	seen := map[string]bool{}
	for id, kind := range portKind {
		if seen[id] {
			continue
		}
		seen[id] = true
		ch := portChamber[id]
		switch kind {
		case PortPressureInlet:
			inletChamber = ch
		case PortPressureSensor:
			sensorChambers = append(sensorChambers, ch)
		}
	}

	for _, p := range in.Pipes {
		addEdge(portChamber[p.From], portChamber[p.To])
	}

	// Ensure every chamber node exists in the adjacency map, even if isolated.
	for _, c := range in.Chambers {
		if adj[c.ID] == nil {
			adj[c.ID] = map[string]bool{}
		}
	}

	// Breadth-first reachability from the inlet chamber.
	reachable := map[string]bool{}
	if inletChamber != "" {
		queue := []string{inletChamber}
		reachable[inletChamber] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for n := range adj[cur] {
				if !reachable[n] {
					reachable[n] = true
					queue = append(queue, n)
				}
			}
		}
	}

	// Every chamber must be reachable from the inlet.
	var disconnected []string
	for _, c := range in.Chambers {
		if !reachable[c.ID] {
			disconnected = append(disconnected, c.ID)
		}
	}
	if len(disconnected) > 0 {
		sort.Strings(disconnected)
		return fmt.Errorf("broken pressure path: chambers not connected to pressure inlet: %v", disconnected)
	}

	// The inlet must reach at least one pressure sensor.
	hasSensor := false
	for _, ch := range sensorChambers {
		if reachable[ch] {
			hasSensor = true
			break
		}
	}
	if inletChamber != "" && !hasSensor {
		return fmt.Errorf("broken pressure path: pressure inlet cannot reach a pressure sensor")
	}
	return nil
}
