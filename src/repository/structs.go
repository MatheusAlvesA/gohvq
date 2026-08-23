package repository

import "sync/atomic"

type QueueHead struct {
	FirstItem *QueueItem
	LastItem  *QueueItem
	Length    atomic.Uint64
	Deleted   atomic.Uint64
}

type QueueItem struct {
	Key       string
	CreatedAt int64
	LastPing  atomic.Int64
	EnterPos  uint64
	Next      *QueueItem
	Previus   *QueueItem
}

func (h *QueueHead) addItem(item *QueueItem) {
	item.Next = nil
	item.Previus = nil
	item.EnterPos = h.Length.Load()
	h.Length.Add(1)
	if h.FirstItem == nil {
		h.FirstItem = item
		h.LastItem = item
		return
	}
	h.LastItem.Next = item
	item.Previus = h.LastItem
	h.LastItem = item
}

func (h *QueueHead) PopItem() *QueueItem {
	if h.FirstItem == nil {
		return nil
	}
	rItem := h.FirstItem
	h.FirstItem = h.FirstItem.Next
	h.Deleted.Add(1)
	if h.FirstItem == nil {
		h.LastItem = nil
		h.Length.Store(0)
		h.Deleted.Store(0)
	} else {
		h.FirstItem.Previus = nil
	}
	return rItem
}
