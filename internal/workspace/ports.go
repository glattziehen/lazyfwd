package workspace

import "fmt"

const (
	// MaxPortDistance bounds how far from a wanted port the search for a free one goes.
	MaxPortDistance       = 1000
	firstUnprivilegedPort = 1024
)

// AssignPorts picks one free port per wanted port. Each wanted port is kept
// when free; the rest get the nearest free port, preferring higher ports on a
// tie. Exact matches are assigned first, so in [4966 4967] with 4966 taken,
// 4967 keeps its port and 4966 moves elsewhere.
func AssignPorts(wanted []int, isFree func(port int) bool) ([]int, error) {
	assigned := make([]int, len(wanted))
	isUsed := map[int]bool{}
	isAvailable := func(port int) bool {
		return port >= 1 && port <= 65535 && !isUsed[port] && isFree(port)
	}
	for i, port := range wanted {
		if isAvailable(port) {
			assigned[i] = port
			isUsed[port] = true
		}
	}
	for i, port := range wanted {
		if assigned[i] != 0 {
			continue
		}
		nearest, ok := nearestAvailable(port, isAvailable)
		if !ok {
			return nil, fmt.Errorf("no free port within %d of %d", MaxPortDistance, port)
		}
		assigned[i] = nearest
		isUsed[nearest] = true
	}
	return assigned, nil
}

func nearestAvailable(port int, isAvailable func(int) bool) (int, bool) {
	floor := 1
	if port >= firstUnprivilegedPort {
		floor = firstUnprivilegedPort
	}
	for distance := 1; distance <= MaxPortDistance; distance++ {
		if up := port + distance; isAvailable(up) {
			return up, true
		}
		if down := port - distance; down >= floor && isAvailable(down) {
			return down, true
		}
	}
	return 0, false
}
