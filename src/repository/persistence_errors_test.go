package repository

import (
	"MatheusAlvesA/gohvq/src/log"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPersistenceFailedMutationsPreserveIndex(t *testing.T) {
	for _, operation := range []struct {
		name     string
		action   int
		finished bool
	}{
		{"add", PersistenceAdd, false},
		{"remove", PersistenceRemove, false},
		{"finish", PersistanceFinish, false},
		{"clear queue", PersistenceClearQueue, false},
		{"clear finished", PersistenceClearFinished, true},
	} {
		t.Run(operation.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			p := InitPersistence()
			var err error
			p.dbFile, err = os.OpenFile(PERSISTENCE_DB_FILE, os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Stop()
			key := strings.Repeat("a", 64)
			if err := p.applyAction(&PersistanceAction{ActionType: PersistenceAdd, Key: key}); err != nil {
				t.Fatal(err)
			}
			if operation.finished {
				if err := p.applyAction(&PersistanceAction{ActionType: PersistanceFinish, Key: key}); err != nil {
					t.Fatal(err)
				}
			}
			beforeItem, beforeCounter := *p.ItemMap[key], p.counter
			beforeDisk := persistenceContents(t)
			if err := p.dbFile.Close(); err != nil {
				t.Fatal(err)
			}
			actionKey := key
			if operation.action == PersistenceAdd {
				actionKey = strings.Repeat("b", 64)
			}
			err = p.applyAction(&PersistanceAction{ActionType: operation.action, Key: actionKey})
			if err == nil {
				t.Fatal("write failure was not returned to the caller")
			}
			if len(p.ItemMap) != 1 || p.ItemMap[key] == nil || !reflect.DeepEqual(*p.ItemMap[key], beforeItem) || p.counter != beforeCounter {
				t.Fatal("failed disk mutation changed the in-memory index")
			}
			if got := persistenceContents(t); got != beforeDisk {
				t.Fatal("failed disk mutation changed the file")
			}
		})
	}
}

func TestPersistenceCompactionCreationFailurePreservesDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	data := "A " + strings.Repeat("a", 64) + "\n"
	if err := os.WriteFile(PERSISTENCE_DB_FILE, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	// A directory at the temporary-file path fails even when tests run as root.
	if err := os.Mkdir(PERSISTENCE_DB_FILE+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	p := InitPersistence()
	p.SetLogService(log.InitService())
	r := InitRepository()
	p.Start(r)
	defer p.Stop()
	if p.active.Load() || p.dbFile != nil || r.GetCurrentQueueSize() != 0 {
		t.Fatal("failed startup enabled persistence or published incomplete state")
	}
	if got := persistenceContents(t); got != data {
		t.Fatal("failed compaction changed the original database")
	}
}

func TestPersistenceRegenerateWithoutOpenFile(t *testing.T) {
	p := InitPersistence()
	r := InitRepository()
	if err := p.Regenerate(r); err == nil {
		t.Fatal("recovery without an open file did not report an error")
	}
	p.Enabled = false
	if err := p.Regenerate(r); err != nil {
		t.Fatalf("disabled recovery should do nothing: %v", err)
	}
	if r.GetCurrentQueueSize() != 0 {
		t.Fatal("recovery without a file changed the queue")
	}
}
