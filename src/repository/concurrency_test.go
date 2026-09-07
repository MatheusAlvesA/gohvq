package repository

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestSnapshotsRemainStableAfterQueueChanges(t *testing.T) {
	r := InitRepository()
	first, _ := r.CreateItem("")
	second, _ := r.CreateItem("")
	position := r.GetAndPingItemByKey(second.Key)
	r.FinishItems(1)
	r.doClear()
	r.FinishItems(1)
	if first.FinishedAt != 0 || second.FinishedAt != 0 || second.Position != 1 ||
		position.FinishedAt != 0 || position.Position != 1 {
		t.Fatal("previous response changed when its internal queue node changed")
	}
	finished := r.GetFinished(second.Key)
	if finished == nil || finished.FinishedAt == 0 || finished.Position != 0 {
		t.Fatal("new snapshot does not reflect the updated node")
	}
	finished.Key = "changed by caller"
	finished.FinishedAt = 0
	if got := r.GetFinished(second.Key); got == nil || got.FinishedAt == 0 || got.Key != second.Key {
		t.Fatal("editing a returned snapshot modified the repository")
	}
}

func TestConcurrentPositionSnapshotsAndCleanup(t *testing.T) {
	r := InitRepository()
	item, _ := r.CreateItem("")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1000 {
			r.doClear()
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			got := r.GetAndPingItemByKey(item.Key)
			if got == nil || got.Position != 0 || got.FinishedAt != 0 {
				t.Error("unexpected position snapshot for the only active item")
				return
			}
		}
	}()
	wg.Wait()
}

func TestConcurrentSizeQueriesAndClear(t *testing.T) {
	r := InitRepository()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1000 {
			r.CreateItem("")
			r.ClearQueue()
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			if size := r.GetCurrentQueueSize(); size > 1 {
				t.Errorf("queue with at most one item reported size %d", size)
				return
			}
		}
	}()
	wg.Wait()
	if r.GetCurrentQueueSize() != 0 {
		t.Fatal("queue not empty after final clear")
	}
}

func TestCleanupRespectsFrequency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := InitRepository()
		r.ClearFrequency = 10
		r.PingTimeout = 1
		r.CreateItem("")
		r.Start()
		defer r.Stop()
		time.Sleep(500 * time.Millisecond)
		synctest.Wait() // First scan, while the item is still alive.
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if r.GetCurrentQueueSize() != 1 {
			t.Fatal("cleanup ran again before ClearFrequency elapsed")
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if r.GetCurrentQueueSize() != 0 {
			t.Fatal("expired item was not removed on the next scheduled cleanup")
		}
	})
}

func TestCleanupTimeoutRespectsFrequency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := InitRepository()
		r.ClearFrequency = 10
		r.ClearMaxTime = 0
		item, _ := r.CreateItem("")
		r.ItemMap[item.Key].LastPing.Store(time.Now().Unix() - 100)
		r.doClear() // Immediately exhaust the time budget.
		r.ClearMaxTime = 1
		r.clearIfDue()
		if r.GetCurrentQueueSize() != 1 {
			t.Fatal("a timed-out scan was retried before ClearFrequency elapsed")
		}
		time.Sleep(10 * time.Second)
		r.clearIfDue()
		if r.GetCurrentQueueSize() != 0 {
			t.Fatal("timed-out scan was not retried when due")
		}
	})
}
