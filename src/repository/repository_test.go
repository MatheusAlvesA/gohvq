package repository

import (
	"testing"
	"time"
)

func TestGenerateRandomKey(t *testing.T) {
	generated, err := GenerateRandomKey()
	if err != nil {
		t.Errorf("Error generating key %q", err)
	}
	if len(generated) != int(KEY_SIZE) {
		t.Errorf("Invalid key size: %d", len(generated))
	}
	if !IsValidKey(generated) {
		t.Errorf("Generated key should be valid: %q", generated)
	}
}

func TestGenerateRandomKeyUniqueness(t *testing.T) {
	keys := map[string]bool{}
	for range 1000 {
		generated, err := GenerateRandomKey()
		if err != nil {
			t.Fatalf("Error generating key %q", err)
		}
		if keys[generated] {
			t.Fatalf("Duplicated key generated: %q", generated)
		}
		keys[generated] = true
	}
}

func TestIsValidKey(t *testing.T) {
	valid := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ab"
	if !IsValidKey(valid) {
		t.Errorf("Key should be valid: %q", valid)
	}

	invalidCases := []string{
		"",
		"short",
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ab!", // invalid char
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789a",   // too short
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abc", // too long
	}
	for _, key := range invalidCases {
		if IsValidKey(key) {
			t.Errorf("Key should be invalid: %q", key)
		}
	}
}

func TestCreateItem(t *testing.T) {
	repo := InitRepository()

	item, err := repo.CreateItem()
	if err != nil {
		t.Fatalf("Error creating item %q", err)
	}
	if !IsValidKey(item.Key) {
		t.Errorf("Created item has invalid key: %q", item.Key)
	}
	if repo.ItemMap[item.Key] != item {
		t.Errorf("Created item not present on ItemMap")
	}
	if repo.Head.FirstItem != item {
		t.Errorf("Created item should be the first on queue")
	}
	if repo.GetCurrentQueueSize() != 1 {
		t.Errorf("Incorrect queue size, expected 1 got %d", repo.GetCurrentQueueSize())
	}

	second, err := repo.CreateItem()
	if err != nil {
		t.Fatalf("Error creating item %q", err)
	}
	if second.Key == item.Key {
		t.Errorf("Created items should have distinct keys")
	}
	if repo.GetCurrentQueueSize() != 2 {
		t.Errorf("Incorrect queue size, expected 2 got %d", repo.GetCurrentQueueSize())
	}
	if repo.Head.FirstItem != item || repo.Head.LastItem != second {
		t.Errorf("Queue order incorrect after second insert")
	}
}

func TestClearQueue(t *testing.T) {
	repo := InitRepository()
	repo.CreateItem()
	repo.CreateItem()

	repo.ClearQueue()

	if repo.GetCurrentQueueSize() != 0 {
		t.Errorf("Queue should be empty after clear, got %d", repo.GetCurrentQueueSize())
	}
	if len(repo.ItemMap) != 0 {
		t.Errorf("ItemMap should be empty after clear, got %d", len(repo.ItemMap))
	}
	if repo.Head.FirstItem != nil || repo.Head.LastItem != nil {
		t.Errorf("Head pointers should be nil after clear")
	}
}

func TestFinishItems(t *testing.T) {
	repo := InitRepository()
	item1, _ := repo.CreateItem()
	item2, _ := repo.CreateItem()
	item3, _ := repo.CreateItem()

	finished := repo.FinishItems(2)

	if len(finished) != 2 {
		t.Fatalf("Expected 2 finished items, got %d", len(finished))
	}
	if finished[0] != item1 || finished[1] != item2 {
		t.Errorf("Finished items should follow queue order (FIFO)")
	}
	if repo.GetCurrentQueueSize() != 1 {
		t.Errorf("Incorrect queue size after finish, expected 1 got %d", repo.GetCurrentQueueSize())
	}
	if repo.ItemMap[item1.Key] != nil || repo.ItemMap[item2.Key] != nil {
		t.Errorf("Finished items should be removed from ItemMap")
	}
	if repo.FinishedMap[item1.Key] != item1 || repo.FinishedMap[item2.Key] != item2 {
		t.Errorf("Finished items should be present on FinishedMap")
	}
	if finished[0].FinishedAt == 0 {
		t.Errorf("Finished item should have FinishedAt set")
	}
	if finished[0].Next != nil || finished[0].Previus != nil {
		t.Errorf("Finished item should not hold queue pointers")
	}
	if repo.Head.FirstItem != item3 {
		t.Errorf("Remaining item should be the queue head")
	}
}

func TestFinishItemsEdgeCases(t *testing.T) {
	repo := InitRepository()

	finished := repo.FinishItems(0)
	if len(finished) != 0 {
		t.Errorf("FinishItems(0) should return empty list")
	}

	repo.CreateItem()
	finished = repo.FinishItems(5)
	if len(finished) != 1 {
		t.Errorf("Finishing more than available should return only existing items, got %d", len(finished))
	}
	if repo.GetCurrentQueueSize() != 0 {
		t.Errorf("Queue should be empty, got %d", repo.GetCurrentQueueSize())
	}
}

func TestGetFinished(t *testing.T) {
	repo := InitRepository()

	if repo.GetFinished("some key") != nil {
		t.Errorf("GetFinished on empty repo should return nil")
	}

	item, _ := repo.CreateItem()
	repo.FinishItems(1)

	if repo.GetFinished(item.Key) != item {
		t.Errorf("GetFinished should return the finished item")
	}
}

func TestDeleteFinished(t *testing.T) {
	repo := InitRepository()

	if repo.DeleteFinished("some key") != nil {
		t.Errorf("DeleteFinished on empty repo should return nil")
	}

	item, _ := repo.CreateItem()
	repo.FinishItems(1)

	deleted := repo.DeleteFinished(item.Key)
	if deleted != item {
		t.Errorf("DeleteFinished should return the deleted item")
	}
	if repo.GetFinished(item.Key) != nil {
		t.Errorf("Item should not be present after delete")
	}
	if repo.DeleteFinished(item.Key) != nil {
		t.Errorf("DeleteFinished on missing key should return nil")
	}
}

func TestClearFinished(t *testing.T) {
	repo := InitRepository()
	repo.CreateItem()
	repo.CreateItem()
	repo.FinishItems(2)

	repo.ClearFinished()

	if len(repo.FinishedMap) != 0 {
		t.Errorf("FinishedMap should be empty after clear, got %d", len(repo.FinishedMap))
	}
}

func TestGetAndPingItemByKey(t *testing.T) {
	repo := InitRepository()

	if repo.GetAndPingItemByKey("some key") != nil {
		t.Errorf("Get on empty repo should return nil")
	}

	item, _ := repo.CreateItem()
	item.LastPing.Store(time.Now().Unix() - 30)

	got := repo.GetAndPingItemByKey(item.Key)
	if got != item {
		t.Fatalf("Get should return the item")
	}
	if got.LastPing.Load() < time.Now().Unix()-5 {
		t.Errorf("Get should update LastPing")
	}
}

func TestDoClear(t *testing.T) {
	repo := InitRepository()
	expired, _ := repo.CreateItem()
	alive, _ := repo.CreateItem()
	expired2, _ := repo.CreateItem()

	expired.LastPing.Store(time.Now().Unix() - PING_TIMEOUT - 1)
	expired2.LastPing.Store(time.Now().Unix() - PING_TIMEOUT - 1)

	repo.doClear()

	if repo.GetCurrentQueueSize() != 1 {
		t.Errorf("Expired items should be removed, queue size expected 1 got %d", repo.GetCurrentQueueSize())
	}
	if repo.ItemMap[expired.Key] != nil || repo.ItemMap[expired2.Key] != nil {
		t.Errorf("Expired items should be removed from ItemMap")
	}
	if repo.ItemMap[alive.Key] != alive {
		t.Errorf("Alive item should remain on ItemMap")
	}
	if repo.Head.FirstItem != alive || repo.Head.LastItem != alive {
		t.Errorf("Alive item should be the only one on queue")
	}
	if alive.Position != 0 {
		t.Errorf("Positions should be recomputed, expected 0 got %d", alive.Position)
	}
}

func TestDoClearEmptyQueue(t *testing.T) {
	repo := InitRepository()
	repo.doClear()

	if repo.GetCurrentQueueSize() != 0 {
		t.Errorf("Queue should remain empty, got %d", repo.GetCurrentQueueSize())
	}
}

func TestStartStop(t *testing.T) {
	repo := InitRepository()
	repo.Start()
	repo.Stop()

	repo2 := InitRepository()
	repo2.Start()
	repo2.Stop()
}

func BenchmarkGenerateRandomKey(b *testing.B) {
	for b.Loop() {
		_, err := GenerateRandomKey()
		if err != nil {
			b.Fatalf("Error generating key %q", err)
		}
	}
}

func BenchmarkIsValidKey(b *testing.B) {
	key := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ab"

	for b.Loop() {
		IsValidKey(key)
	}
}

func BenchmarkCreateItem(b *testing.B) {
	repo := InitRepository()

	for b.Loop() {
		_, err := repo.CreateItem()
		if err != nil {
			b.Fatalf("Error creating item %q", err)
		}
	}
}

func BenchmarkGetAndPingItemByKey(b *testing.B) {
	repo := InitRepository()
	item, _ := repo.CreateItem()

	for b.Loop() {
		repo.GetAndPingItemByKey(item.Key)
	}
}

func BenchmarkFinishItems(b *testing.B) {
	repo := InitRepository()

	for b.Loop() {
		repo.CreateItem()
		repo.FinishItems(1)
	}
}

func BenchmarkDoClear(b *testing.B) {
	repo := InitRepository()
	for range 1000 {
		repo.CreateItem()
	}

	for b.Loop() {
		repo.doClear()
	}
}
