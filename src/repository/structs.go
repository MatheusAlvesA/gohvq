package repository

import "sync/atomic"

const (
	PersistenceAdd           = 1
	PersistenceRemove        = 2
	PersistanceFinish        = 3
	PersistenceClearQueue    = 4
	PersistenceClearFinished = 5
)

type QueueHead struct {
	FirstItem *QueueItem
	LastItem  *QueueItem
	Length    atomic.Uint64
}

type QueueItem struct {
	Key        string
	CreatedAt  int64
	LastPing   atomic.Int64
	Position   uint64
	FinishedAt int64
	Next       *QueueItem
	Previus    *QueueItem
}

// ItemSnapshot contains values captured under the repository lock. It does not
// expose the mutable queue node or copy its atomic fields.
type ItemSnapshot struct {
	Key        string
	CreatedAt  int64
	LastPing   int64
	Position   uint64
	FinishedAt int64
}

func (i *QueueItem) snapshot() *ItemSnapshot {
	if i == nil {
		return nil
	}
	return &ItemSnapshot{
		Key:        i.Key,
		CreatedAt:  i.CreatedAt,
		LastPing:   i.LastPing.Load(),
		Position:   i.Position,
		FinishedAt: i.FinishedAt,
	}
}

type PersistanceAction struct {
	ActionType int
	Key        string
}

type PersistenceItem struct {
	Key      string
	Index    uint64
	Finished bool
}

func (h *QueueHead) AddItem(item *QueueItem) {
	item.Next = nil
	item.Previus = nil
	item.Position = h.Length.Load()
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
	h.Length.Store(h.Length.Load() - 1)
	if h.FirstItem == nil {
		h.LastItem = nil
		h.Length.Store(0)
	} else {
		h.FirstItem.Previus = nil
	}
	return rItem
}

func (h *QueueHead) Detach(i *QueueItem) {
	h.Length.Store(h.Length.Load() - 1)
	if i.Previus != nil {
		i.Previus.Next = i.Next
	}
	if i.Next != nil {
		i.Next.Previus = i.Previus
	}
	if h.FirstItem == i {
		h.FirstItem = i.Next
	}
	if h.LastItem == i {
		h.LastItem = i.Previus
	}
	i.Next = nil
	i.Previus = nil
}
