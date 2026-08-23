package repository

type QueueHead struct {
	firstItem *QueueItem
	lastItem  *QueueItem
	length    uint64
}

type QueueItem struct {
	Key  string
	Next *QueueItem
}

func (h *QueueHead) addItem(item *QueueItem) {
	if h.firstItem == nil {
		h.firstItem = item
		h.lastItem = item
		return
	}
	h.lastItem.Next = item
	h.lastItem = item
	h.length++
}

func (h *QueueHead) popItem() *QueueItem {
	if h.firstItem == nil {
		return nil
	}
	rItem := h.firstItem
	h.firstItem = h.firstItem.Next
	h.length--
	if h.firstItem == nil {
		h.lastItem = nil
		h.length = 0
	}
	return rItem
}
