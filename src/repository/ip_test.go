package repository

import (
	"strings"
	"testing"
)

func TestPersistenceIPRoundTripAndOffsets(t *testing.T) {
	t.Chdir(t.TempDir())
	r := startPersistentRepository(t)
	ips := []string{"192.0.2.1", "2001:db8::1", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"}
	var keys []string
	for _, ip := range ips {
		item, err := r.CreateItem(ip)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, item.Key)
	}
	r.FinishItems(1)
	r.Persistence.Stop()
	data := persistenceContents(t)
	if len(data) != 3*int(PERSISTANCE_DB_LINE_SIZE) {
		t.Fatal("variable record lengths")
	}
	for i, line := range strings.Split(strings.TrimSuffix(data, "\n"), "\n") {
		if len(line)+1 != int(PERSISTANCE_DB_LINE_SIZE) || line[3+int(KEY_SIZE):] != ips[i]+strings.Repeat(" ", PERSISTENCE_IP_SIZE-len(ips[i])) {
			t.Fatalf("invalid padded record %q", line)
		}
	}
	restored := startPersistentRepository(t)
	if restored.GetFinished(keys[0]).IP != ips[0] {
		t.Fatal("finished IP lost")
	}
	for i := 1; i < len(keys); i++ {
		if restored.GetAndPingItemByKey(keys[i]).IP != ips[i] {
			t.Fatal("active IP lost")
		}
	}
	restored.DeleteFinished(keys[0])
	restored.FinishItems(1)
	restored.Persistence.Stop()
	again := startPersistentRepository(t)
	if again.GetFinished(keys[0]) != nil || again.GetFinished(keys[1]).IP != ips[1] || again.GetAndPingItemByKey(keys[2]).IP != ips[2] {
		t.Fatal("offset mutation or compaction lost IP")
	}
}

func TestInvalidIPDoesNotCreateTicket(t *testing.T) {
	r := InitRepository()
	if _, err := r.CreateItem("invalid\nIP"); err == nil || r.GetCurrentQueueSize() != 0 {
		t.Fatal("invalid IP accepted or queue mutated")
	}
}
