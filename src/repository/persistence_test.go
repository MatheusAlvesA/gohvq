package repository

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func startPersistentRepository(t *testing.T) *Repository {
	t.Helper()
	r := InitRepository()
	r.Persistence = InitPersistence()
	r.Persistence.Start(r)
	t.Cleanup(r.Persistence.Stop)
	if !r.Persistence.active.Load() {
		t.Fatal("persistence did not start")
	}
	return r
}

func persistenceContents(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(PERSISTENCE_DB_FILE)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPersistenceFinishSurvivesRestart(t *testing.T) {
	t.Chdir(t.TempDir())
	r := startPersistentRepository(t)
	item, err := r.CreateItem()
	if err != nil {
		t.Fatal(err)
	}
	r.FinishItems(1)
	r.Persistence.Stop() // Must drain both add and finish, without sleeps.
	if got := persistenceContents(t); got != "F "+item.Key+"\n" {
		t.Fatalf("finished record not saved: %q", got)
	}

	restored := startPersistentRepository(t)
	if restored.GetCurrentQueueSize() != 0 || restored.GetFinished(item.Key) == nil {
		t.Fatal("finished item returned to the active queue on restart")
	}
	restored.DeleteFinished(item.Key)
	restored.Persistence.Stop()
	if got := persistenceContents(t); got != "X "+item.Key+"\n" {
		t.Fatalf("restored finished item was not deleted: %q", got)
	}
}

func TestPersistenceClearOrdersPendingActions(t *testing.T) {
	for _, finished := range []bool{false, true} {
		name := "queue"
		if finished {
			name = "finished"
		}
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			r := startPersistentRepository(t)
			var keepQueue, keepFinished *ItemSnapshot
			// Hold the disk worker back, so none of the queued actions can have
			// updated its index when the repository issues the clear operation.
			func() {
				r.Persistence.mutex.Lock()
				defer r.Persistence.mutex.Unlock()
				first, _ := r.CreateItem()
				r.FinishItems(1)
				second, _ := r.CreateItem()
				if finished {
					r.ClearFinished()
					r.FinishItems(1) // A later finish must survive the clear.
					keepFinished = second
				} else {
					r.ClearQueue()
					keepFinished = first
				}
				keepQueue, _ = r.CreateItem() // A later add must also survive.
			}()
			r.Persistence.Stop()
			restored := startPersistentRepository(t)
			if restored.GetCurrentQueueSize() != 1 || restored.Head.FirstItem.Key != keepQueue.Key {
				t.Fatal("clear lost a later add or restored a removed queue item")
			}
			if len(restored.FinishedMap) != 1 || restored.GetFinished(keepFinished.Key) == nil {
				t.Fatal("clear lost a retained finish or restored a removed finished item")
			}
		})
	}
}

func TestPersistenceRecoversPartialTail(t *testing.T) {
	for _, tail := range []string{"A", "A " + strings.Repeat("d", int(KEY_SIZE))} {
		t.Run(strconv.Itoa(len(tail)), func(t *testing.T) {
			t.Chdir(t.TempDir())
			deleted, queued, finished := strings.Repeat("a", int(KEY_SIZE)), strings.Repeat("b", int(KEY_SIZE)), strings.Repeat("c", int(KEY_SIZE))
			data := "X " + deleted + "\nA " + queued + "\nF " + finished + "\n" + tail
			if err := os.WriteFile(PERSISTENCE_DB_FILE, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			r := startPersistentRepository(t)
			if r.GetCurrentQueueSize() != 1 || r.Head.FirstItem.Key != queued || r.GetFinished(finished) == nil {
				t.Fatal("valid records preceding the partial tail were not restored")
			}
			if got := persistenceContents(t); got != "A "+queued+"\nF "+finished+"\n" {
				t.Fatalf("partial tail or tombstone survived compaction: %q", got)
			}
			r.DeleteFinished(finished)
			r.FinishItems(1)
			added, _ := r.CreateItem()
			r.Persistence.Stop()
			want := "F " + queued + "\nX " + finished + "\nA " + added.Key + "\n"
			if got := persistenceContents(t); got != want {
				t.Fatalf("writes used incorrect offsets after recovery:\n got %q\nwant %q", got, want)
			}
			restored := startPersistentRepository(t)
			if restored.GetCurrentQueueSize() != 1 || restored.Head.FirstItem.Key != added.Key ||
				len(restored.FinishedMap) != 1 || restored.GetFinished(queued) == nil {
				t.Fatal("repaired database did not survive another restart")
			}
		})
	}
}

func TestPersistenceRejectsInvalidRecordWithoutPartialRestore(t *testing.T) {
	t.Chdir(t.TempDir())
	data := "A " + strings.Repeat("a", int(KEY_SIZE)) + "\n? " + strings.Repeat("b", int(KEY_SIZE)) + "\n"
	if err := os.WriteFile(PERSISTENCE_DB_FILE, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	r := InitRepository()
	r.Persistence = InitPersistence()
	r.Persistence.Start(r)
	defer r.Persistence.Stop()
	if r.Persistence.active.Load() || r.GetCurrentQueueSize() != 0 {
		t.Fatal("failed recovery published partial state or enabled the worker")
	}
	if got := persistenceContents(t); got != data {
		t.Fatal("failed recovery modified the original file")
	}
}

func TestPersistenceOpenFailureDoesNotBlockRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir(PERSISTENCE_DB_FILE, 0700); err != nil {
		t.Fatal(err)
	}
	r := InitRepository()
	r.Persistence = InitPersistence()
	r.Persistence.Start(r)
	defer r.Persistence.Stop()
	key := strings.Repeat("a", int(KEY_SIZE))
	for range PERSISTENCE_ACTIONS_BUFFER_SIZE + 1 {
		r.Persistence.AddItem(key)
	}
	r.RegenerateQueueItem(key)
	done := make(chan struct{})
	go func() {
		r.FinishItems(1)
		r.ClearQueue()
		r.ClearFinished()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("repository blocked after persistence failed to open")
	}
	if r.Persistence.active.Load() || len(r.Persistence.actions) != 0 {
		t.Fatal("actions queued without an active consumer")
	}
}

func TestPersistenceWriteFailureReleasesProducers(t *testing.T) {
	t.Chdir(t.TempDir())
	r := InitRepository()
	p := InitPersistence()
	p.actions = make(chan *PersistanceAction, 1)
	p.Start(r)
	defer p.Stop()
	// Simulate an I/O failure while producers contend for a tiny buffer.
	p.mutex.Lock()
	if err := p.dbFile.Close(); err != nil {
		p.mutex.Unlock()
		t.Fatal(err)
	}
	p.AddItem(strings.Repeat("a", int(KEY_SIZE)))
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.FinishItem(strings.Repeat("a", int(KEY_SIZE)))
			p.ClearAllNotFinished()
			p.ClearAllFinished()
		}()
	}
	p.mutex.Unlock()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("producers remained blocked after the consumer failed")
	}
	p.Stop()
	if p.active.Load() {
		t.Fatal("failed persistence is still active")
	}
}

func TestDisabledPersistenceDoesNotTouchDisk(t *testing.T) {
	t.Chdir(t.TempDir())
	data := "A " + strings.Repeat("a", int(KEY_SIZE)) + "\n"
	if err := os.WriteFile(PERSISTENCE_DB_FILE, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	r := InitRepository()
	r.Persistence = InitPersistence()
	r.Persistence.Enabled = false
	r.Persistence.Start(r)
	r.CreateItem()
	r.FinishItems(1)
	r.ClearQueue()
	r.ClearFinished()
	r.Persistence.Stop()
	if got := persistenceContents(t); got != data {
		t.Fatal("disabled persistence changed the database")
	}
}

func TestPersistenceConcurrentClearsAndWrites(t *testing.T) {
	t.Chdir(t.TempDir())
	r := startPersistentRepository(t)
	var wg sync.WaitGroup
	for _, work := range []func(){
		func() { r.CreateItem(); r.FinishItems(1) },
		func() { r.CreateItem() },
		r.ClearQueue,
		r.ClearFinished,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				work()
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent clearing and writing deadlocked")
	}
	r.Persistence.Stop()
	restored := startPersistentRepository(t)
	queueKeys := func(repo *Repository) []string {
		var keys []string
		for node := repo.Head.FirstItem; node != nil; node = node.Next {
			keys = append(keys, node.Key)
		}
		return keys
	}
	if !reflect.DeepEqual(queueKeys(r), queueKeys(restored)) {
		t.Fatal("restored FIFO queue differs from memory after concurrent operations")
	}
	if len(r.FinishedMap) != len(restored.FinishedMap) {
		t.Fatal("restored finished count differs from memory")
	}
	for key := range r.FinishedMap {
		if restored.GetFinished(key) == nil {
			t.Fatal("restored finished items differ from memory")
		}
	}
}
