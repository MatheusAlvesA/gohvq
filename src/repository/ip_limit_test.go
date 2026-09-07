package repository

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestIPLimitLifecycle(t *testing.T) {
	for _, exit := range []string{"finish", "expire", "clear"} {
		t.Run(exit, func(t *testing.T) {
			r := InitRepository()
			r.MaxEntriesPerIP = 1
			first, err := r.CreateItem("192.0.2.1")
			if err != nil {
				t.Fatal(err)
			}
			if item, err := r.CreateItem("::ffff:192.0.2.1"); item != nil || !errors.Is(err, ErrIPLimitReached) || r.GetCurrentQueueSize() != 1 {
				t.Fatal("limit did not reject without mutation")
			}
			if _, err := r.CreateItem("192.0.2.2"); err != nil {
				t.Fatal(err)
			}
			switch exit {
			case "finish":
				r.FinishItems(1)
				r.DeleteFinished(first.Key)
				r.ClearFinished()
			case "expire":
				r.ItemMap[first.Key].LastPing.Store(0)
				r.doClear()
			case "clear":
				r.ClearQueue()
			}
			if _, ok := r.ipCounts["192.0.2.1"]; ok {
				t.Fatal("unused IP retained")
			}
			if _, err := r.CreateItem("192.0.2.1"); err != nil {
				t.Fatal(err)
			}
			if _, err := r.CreateItem("192.0.2.1"); !errors.Is(err, ErrIPLimitReached) {
				t.Fatal("limit not restored")
			}
		})
	}
}

func TestIPLimitConcurrentEntries(t *testing.T) {
	r := InitRepository()
	r.MaxEntriesPerIP = 7
	var wg sync.WaitGroup
	var accepted atomic.Uint64
	for range 100 {
		wg.Go(func() {
			if _, err := r.CreateItem("2001:db8::1"); err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrIPLimitReached) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 7 || r.GetCurrentQueueSize() != 7 || r.ipCounts["2001:db8::1"] != 7 {
		t.Fatal("concurrent entries exceeded limit or corrupted count")
	}
}

func TestIPLimitRecovery(t *testing.T) {
	t.Chdir(t.TempDir())
	original := startPersistentRepository(t)
	for range 4 {
		if _, err := original.CreateItem("192.0.2.1"); err != nil {
			t.Fatal(err)
		}
	}
	original.FinishItems(1)
	original.Persistence.Stop()
	restored := InitRepository()
	restored.MaxEntriesPerIP = 2
	restored.Persistence = InitPersistence()
	restored.Persistence.Enabled = true
	restored.Persistence.Start(restored)
	t.Cleanup(restored.Persistence.Stop)
	if restored.GetCurrentQueueSize() != 3 || restored.ipCounts["192.0.2.1"] != 3 || len(restored.FinishedMap) != 1 {
		t.Fatal("recovery did not restore counts above limit")
	}
	for range 2 {
		if _, err := restored.CreateItem("192.0.2.1"); !errors.Is(err, ErrIPLimitReached) {
			t.Fatal("recovered count not enforced")
		}
		restored.FinishItems(1)
	}
	if _, err := restored.CreateItem("192.0.2.1"); err != nil {
		t.Fatal(err)
	}
}

func TestIPLimitRegenerationCanonicalizesIP(t *testing.T) {
	r := InitRepository()
	r.MaxEntriesPerIP = 1
	key, err := GenerateRandomKey()
	if err != nil {
		t.Fatal(err)
	}
	r.RegenerateQueueItem(key, "::ffff:192.0.2.1")
	if _, err := r.CreateItem("192.0.2.1"); !errors.Is(err, ErrIPLimitReached) {
		t.Fatal("recovered IP bypassed limit")
	}
	r.FinishItems(1)
	if len(r.ipCounts) != 0 {
		t.Fatal("recovered IP count was not released")
	}
}
