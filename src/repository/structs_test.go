package repository

import (
	"testing"
)

func TestAddItemToHead(t *testing.T) {
	head := QueueHead{}

	head.AddItem(&QueueItem{
		Key: "TestKey",
	})

	if head.FirstItem == nil {
		t.Fatal("First item not present")
	}
	if head.FirstItem.Key != "TestKey" {
		t.Errorf("Incorrect item Key %q", head.FirstItem.Key)
	}

	if head.Length.Load() != 1 {
		t.Errorf("Incorrect size, expected 1 receive %d", head.Length.Load())
	}

	head.AddItem(&QueueItem{
		Key: "TestKey",
	})
	if head.Length.Load() != 2 {
		t.Errorf("Incorrect size, expected 2 receive %d", head.Length.Load())
	}
}

func BenchmarkAddItemToHead(b *testing.B) {
	head := &QueueHead{}

	for b.Loop() {
		head.AddItem(&QueueItem{
			Key: "TestKey",
		})
	}
}
