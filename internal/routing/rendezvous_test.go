package routing

import (
	"fmt"
	"math"
	"net"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestWeightedRendezvousDeterministicAndOrderIndependent(t *testing.T) {
	candidates := []Candidate{{"a", 70}, {"b", 20}, {"c", 10}}
	want, ok := SelectWeightedRendezvous("client|video.example.|1", candidates)
	if !ok {
		t.Fatal("no candidate selected")
	}
	for i := 0; i < 100; i++ {
		got, _ := SelectWeightedRendezvous("client|video.example.|1", candidates)
		if got != want {
			t.Fatalf("selection changed: %s != %s", got, want)
		}
	}
	slices.Reverse(candidates)
	got, _ := SelectWeightedRendezvous("client|video.example.|1", candidates)
	if got != want {
		t.Fatalf("candidate order changed selection: %s != %s", got, want)
	}
}

func TestWeightedRendezvousDistribution(t *testing.T) {
	candidates := []Candidate{{"a", 70}, {"b", 20}, {"c", 10}}
	counts := map[string]int{}
	const samples = 100_000
	for i := 0; i < samples; i++ {
		selected, ok := SelectWeightedRendezvous(fmt.Sprintf("client-%d", i), candidates)
		if !ok {
			t.Fatal("no candidate selected")
		}
		counts[selected]++
	}
	for node, expected := range map[string]float64{"a": .70, "b": .20, "c": .10} {
		actual := float64(counts[node]) / samples
		if math.Abs(actual-expected) > .015 {
			t.Fatalf("node %s distribution %.4f, expected %.2f", node, actual, expected)
		}
	}
}

func TestWeightedRendezvousRemovalHasMinimalDisruption(t *testing.T) {
	before := []Candidate{{"a", 70}, {"b", 20}, {"c", 10}}
	after := []Candidate{{"a", 70}, {"b", 20}}
	const samples = 50_000
	moved, removedWinner := 0, 0
	for i := 0; i < samples; i++ {
		key := fmt.Sprintf("key-%d", i)
		oldNode, _ := SelectWeightedRendezvous(key, before)
		newNode, _ := SelectWeightedRendezvous(key, after)
		if oldNode == "c" {
			removedWinner++
		}
		if oldNode != newNode {
			moved++
			if oldNode != "c" {
				t.Fatalf("key moved between surviving nodes: %s -> %s", oldNode, newNode)
			}
		}
	}
	if moved != removedWinner {
		t.Fatalf("moved=%d removed-winner=%d", moved, removedWinner)
	}
}

func TestWeightedRendezvousRejectsNonPositiveAndInvalidWeights(t *testing.T) {
	candidates := []Candidate{{"zero", 0}, {"negative", -1}, {"nan", math.NaN()}, {"infinite", math.Inf(1)}, {"healthy", 1}}
	for i := 0; i < 100; i++ {
		got, ok := SelectWeightedRendezvous(fmt.Sprintf("key-%d", i), candidates)
		if !ok || got != "healthy" {
			t.Fatalf("selected %q ok=%v", got, ok)
		}
	}
	if _, ok := SelectWeightedRendezvous("key", candidates[:4]); ok {
		t.Fatal("all-zero/invalid candidates did not fail")
	}
}

func TestBuildRoutingKeyNormalizesSubnetAndName(t *testing.T) {
	one := BuildRoutingKey(net.ParseIP("192.0.2.10"), "VIDEO.Example", 1)
	two := BuildRoutingKey(net.ParseIP("192.0.2.240"), "video.example.", 1)
	if one != two || one != "192.0.2.0/24|video.example.|1" {
		t.Fatalf("IPv4 keys differ: %q %q", one, two)
	}
	v6one := BuildRoutingKey(net.ParseIP("2001:db8:abcd:1200::1"), "v.example.", 28)
	v6two := BuildRoutingKey(net.ParseIP("2001:db8:abcd:12ff::2"), "v.example.", 28)
	if v6one != v6two {
		t.Fatalf("IPv6 /56 keys differ: %q %q", v6one, v6two)
	}
}

func TestSnapshotIsImmutableAndConcurrent(t *testing.T) {
	entries := []NodeQuality{{Location: "sydney", Node: "a", EffectiveWeight: 70, State: "Healthy"}}
	snapshot := NewSnapshot(1, time.Now(), entries)
	entries[0].EffectiveWeight = 1
	quality, ok := snapshot.Lookup("sydney", "a")
	if !ok || quality.EffectiveWeight != 70 || snapshot.Len() != 1 {
		t.Fatalf("snapshot changed: %+v", quality)
	}
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 10_000; j++ {
				got, found := snapshot.Lookup("sydney", "a")
				if !found || got.EffectiveWeight != 70 {
					t.Errorf("concurrent read failed: %+v", got)
					return
				}
			}
		}()
	}
	wait.Wait()
}
