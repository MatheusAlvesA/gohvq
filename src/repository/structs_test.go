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

func TestPopItemFromHead(t *testing.T) {
	head := QueueHead{}

	if head.PopItem() != nil {
		t.Errorf("Empty head shoud not return item on pop")
	}

	head.AddItem(&QueueItem{
		Key: "TestKey",
	})

	poped := head.PopItem()
	if poped.Key != "TestKey" {
		t.Errorf("Incorrect item Poped, expect TestKey, got: %q", poped.Key)
	}
	if head.FirstItem != nil || head.Length.Load() != 0 {
		t.Errorf("First item shoud be empty after pop the only item")
	}

	head.AddItem(&QueueItem{
		Key: "TestKey 1",
	})
	head.AddItem(&QueueItem{
		Key: "TestKey 2",
	})

	poped = head.PopItem()
	if poped.Key != "TestKey 1" {
		t.Errorf("Incorrect item Poped on 2 items queue, expect TestKey 1, got: %q", poped.Key)
	}
	if head.FirstItem.Key != "TestKey 2" || head.Length.Load() != 1 {
		t.Errorf("First item shoud be TestKey 1 after pop")
	}
}

func TestDetachItemFromHead(t *testing.T) {
	head := QueueHead{}

	head.AddItem(&QueueItem{
		Key: "TestKey 1",
	})
	toDetach := &QueueItem{
		Key: "TestKey 2",
	}
	head.AddItem(toDetach)
	head.AddItem(&QueueItem{
		Key: "TestKey 3",
	})

	head.Detach(toDetach)
	if toDetach.Next != nil || toDetach.Previus != nil {
		t.Errorf("Detached item shoud not houd pointers")
	}
	if head.FirstItem.Key != "TestKey 1" {
		t.Errorf("First item shoud be TestKey 1 after detach")
	}
	if head.LastItem.Key != "TestKey 3" {
		t.Errorf("Last item shoud be TestKey 3 after detach")
	}
}
func TestDetachFirstItemFromHead(t *testing.T) {
	head := QueueHead{}

	toDetach := &QueueItem{
		Key: "TestKey 1",
	}
	head.AddItem(toDetach)
	head.AddItem(&QueueItem{
		Key: "TestKey 2",
	})
	head.AddItem(&QueueItem{
		Key: "TestKey 3",
	})

	head.Detach(toDetach)
	if toDetach.Next != nil || toDetach.Previus != nil {
		t.Errorf("Detached item shoud not houd pointers")
	}
	if head.FirstItem.Key != "TestKey 2" {
		t.Errorf("First item shoud be TestKey 2 after detach first")
	}
	if head.LastItem.Key != "TestKey 3" {
		t.Errorf("Last item shoud be TestKey 3 after detach first")
	}
}
func TestDetachLastItemFromHead(t *testing.T) {
	head := QueueHead{}

	head.AddItem(&QueueItem{
		Key: "TestKey 1",
	})
	toDetach := &QueueItem{
		Key: "TestKey 3",
	}
	head.AddItem(&QueueItem{
		Key: "TestKey 2",
	})
	head.AddItem(toDetach)

	head.Detach(toDetach)
	if toDetach.Next != nil || toDetach.Previus != nil {
		t.Errorf("Detached item shoud not houd pointers")
	}
	if head.FirstItem.Key != "TestKey 1" {
		t.Errorf("First item shoud be TestKey 1 after detach last")
	}
	if head.LastItem.Key != "TestKey 2" {
		t.Errorf("Last item shoud be TestKey 2 after detach last")
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
