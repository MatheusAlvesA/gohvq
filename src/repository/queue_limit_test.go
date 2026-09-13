package repository

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestQueueLimitLifecycle(t *testing.T) {
	for _, departure := range []string{"finish", "expire", "clear"} {
		t.Run(departure, func(t *testing.T) {
			r := InitRepository()
			r.MaxQueueSize = 2
			first, err := r.CreateItem("192.0.2.1")
			if err != nil {
				t.Fatal(err)
			}
			second, err := r.CreateItem("192.0.2.2")
			if err != nil {
				t.Fatal(err)
			}
			if item, err := r.CreateItem("192.0.2.3"); item != nil || !errors.Is(err, ErrQueueFull) {
				t.Fatalf("full queue: item=%v error=%v", item, err)
			}
			if r.GetCurrentQueueSize() != 2 || len(r.ItemMap) != 2 || len(r.ipCounts) != 2 || r.Head.FirstItem.Key != first.Key || r.Head.LastItem.Key != second.Key {
				t.Fatal("rejection changed queue state")
			}
			switch departure {
			case "finish":
				r.FinishItems(1)
			case "expire":
				r.ItemMap[first.Key].LastPing.Store(0)
				r.doClear()
			case "clear":
				r.ClearQueue()
			}
			item, err := r.CreateItem("192.0.2.3")
			if err != nil {
				t.Fatal(err)
			}
			if departure == "finish" && (item.Position != 2 || r.GetCurrentQueueSize() != 2) {
				t.Fatal("expected admission despite stale positions")
			}
		})
	}
}

func TestQueueLimitConcurrentEntries(t *testing.T) {
	for _, limit := range []uint64{0, 7} {
		r := InitRepository()
		r.MaxQueueSize = limit
		var wg sync.WaitGroup
		var accepted atomic.Uint64
		for range 100 {
			wg.Go(func() {
				if _, err := r.CreateItem(""); err == nil {
					accepted.Add(1)
				} else if !errors.Is(err, ErrQueueFull) {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		want := limit
		if limit == 0 {
			want = 100
		}
		if accepted.Load() != want || r.GetCurrentQueueSize() != want || uint64(len(r.ItemMap)) != want {
			t.Fatalf("limit %d: accepted=%d size=%d", limit, accepted.Load(), r.GetCurrentQueueSize())
		}
	}
}

func TestQueueLimitRecovery(t *testing.T) {
	t.Chdir(t.TempDir())
	original := startPersistentRepository(t)
	for range 3 {
		if _, err := original.CreateItem(""); err != nil {
			t.Fatal(err)
		}
	}
	original.Persistence.Stop()
	r := InitRepository()
	r.MaxQueueSize = 2
	r.Persistence = InitPersistence()
	r.Persistence.Enabled = true
	r.Persistence.Start(r)
	t.Cleanup(r.Persistence.Stop)
	if r.GetCurrentQueueSize() != 3 {
		t.Fatal("recovery discarded tickets above limit")
	}
	for range 2 {
		if _, err := r.CreateItem(""); !errors.Is(err, ErrQueueFull) {
			t.Fatal("recovered capacity not enforced")
		}
		r.FinishItems(1)
	}
	if _, err := r.CreateItem(""); err != nil {
		t.Fatal(err)
	}
}
