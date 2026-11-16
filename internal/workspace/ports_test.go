package workspace

import (
	"slices"
	"testing"
)

func takenPorts(ports ...int) func(int) bool {
	return func(port int) bool { return !slices.Contains(ports, port) }
}

func TestAssignPortsKeepsFreePorts(t *testing.T) {
	got, err := AssignPorts([]int{4966, 4967, 4968}, takenPorts())
	if err != nil || !slices.Equal(got, []int{4966, 4967, 4968}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestAssignPortsMovesTakenPortWithoutStealingSiblings(t *testing.T) {
	got, err := AssignPorts([]int{4966, 4967, 4968}, takenPorts(4966))
	if err != nil || !slices.Equal(got, []int{4965, 4967, 4968}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestAssignPortsPrefersHigherPortOnTie(t *testing.T) {
	got, err := AssignPorts([]int{8080}, takenPorts(8080))
	if err != nil || !slices.Equal(got, []int{8081}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestAssignPortsStaysUnprivileged(t *testing.T) {
	got, err := AssignPorts([]int{1024}, takenPorts(1024, 1025))
	if err != nil || !slices.Equal(got, []int{1026}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestAssignPortsFailsWhenNothingIsFree(t *testing.T) {
	if _, err := AssignPorts([]int{5000}, func(int) bool { return false }); err == nil {
		t.Fatal("want error")
	}
}
