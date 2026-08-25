package repository

import (
	"crypto/rand"
	"math/big"
	"sync"
	"time"
)

const KEY_SIZE uint = 64
const PING_TIMEOUT int64 = 60
const MIN_PING_TIMEOUT int64 = 10
const CLEAR_MAX_TIME uint = 1
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Repository struct {
	Head        *QueueHead
	ItemMap     map[string]*QueueItem
	FinishedMap map[string]*QueueItem
	lastClear   int64
	stopSignal  bool
	wg          sync.WaitGroup
	mu          sync.RWMutex
	muFinished  sync.RWMutex
}

func generateRandomKey() (string, error) {
	result := make([]byte, KEY_SIZE)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

func IsValidKey(key string) bool {
	if len(key) != int(KEY_SIZE) {
		return false
	}
	for _, c := range key {
		n := int(c)
		if !(n >= 48 && n <= 57) && // 0..9
			!(n >= 65 && n <= 90) && // A..Z
			!(n >= 97 && n <= 122) { // a..z
			return false
		}
	}

	return true
}

func (r *Repository) CreateItem() (*QueueItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	selectedKey := ""
	for selectedKey == "" || r.ItemMap[selectedKey] != nil {
		newKey, err := generateRandomKey()
		if err != nil {
			return nil, err
		}
		selectedKey = newKey
	}

	newItem := new(QueueItem)
	newItem.Key = selectedKey
	newItem.CreatedAt = time.Now().Unix()
	newItem.LastPing.Store(time.Now().Unix())
	r.Head.addItem(newItem)
	r.ItemMap[selectedKey] = newItem
	return newItem, nil
}

func (r *Repository) GetCurrentQueueSize() uint64 {
	return r.Head.Length.Load()
}

func (r *Repository) ClearQueue() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Head = &QueueHead{}
	r.ItemMap = map[string]*QueueItem{}
}

func (r *Repository) ClearFinished() {
	r.muFinished.Lock()
	defer r.muFinished.Unlock()
	r.FinishedMap = map[string]*QueueItem{}
}

func (r *Repository) FinishItems(n uint) []*QueueItem {
	if n == 0 {
		return make([]*QueueItem, 0)
	}
	r.mu.Lock()
	r.muFinished.Lock()
	defer r.mu.Unlock()
	defer r.muFinished.Unlock()

	var resultList []*QueueItem
	current := r.Head.PopItem()
	now := time.Now().Unix()
	for current != nil {
		current.Next = nil
		current.Previus = nil
		current.FinishedAt = now
		delete(r.ItemMap, current.Key)
		r.FinishedMap[current.Key] = current
		resultList = append(resultList, current)
		n--
		if n <= 0 {
			break
		}
		current = r.Head.PopItem()
	}

	return resultList
}

func (r *Repository) GetFinished(key string) *QueueItem {
	r.muFinished.RLock()
	defer r.muFinished.RUnlock()

	return r.FinishedMap[key]
}

func (r *Repository) DeleteFinished(key string) *QueueItem {
	r.muFinished.Lock()
	defer r.muFinished.Unlock()

	item := r.FinishedMap[key]
	if item != nil {
		delete(r.FinishedMap, key)
	}
	return item
}

func (r *Repository) GetAndPingItemByKey(key string) *QueueItem {
	r.mu.RLock()
	item := r.ItemMap[key]
	r.mu.RUnlock()
	if item == nil {
		return nil
	}

	item.LastPing.Store(time.Now().Unix())
	return item
}

func (r *Repository) clearTask() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	defer r.wg.Done()

	for true {
		<-ticker.C
		if r.stopSignal {
			return
		}
		now := time.Now().Unix()
		if (now - r.lastClear) < min(PING_TIMEOUT, MIN_PING_TIMEOUT) {
			continue
		}
		r.doClear()
	}
}
func (r *Repository) doClear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().Unix()

	currentItem := r.Head.FirstItem
	timeout := time.NewTimer(time.Duration(CLEAR_MAX_TIME) * time.Second)
	var currentPosition uint64 = 0
	for currentItem != nil {
		currentItem.Position = currentPosition
		select {
		case <-timeout.C:
			return
		default:
			elapsedTimeSeconds := now - currentItem.LastPing.Load()
			if elapsedTimeSeconds <= PING_TIMEOUT {
				currentItem = currentItem.Next
				currentPosition++
				continue
			}
			tmpNext := currentItem.Next
			r.Head.Detach(currentItem)
			delete(r.ItemMap, currentItem.Key)
			currentItem = tmpNext
		}
	}
}

func (r *Repository) Start() {
	r.wg.Add(1)
	go r.clearTask()
}
func (r *Repository) Stop() {
	r.stopSignal = true
	r.wg.Wait()
}

func InitRepository() *Repository {
	return &Repository{
		Head:        &QueueHead{},
		ItemMap:     map[string]*QueueItem{},
		FinishedMap: map[string]*QueueItem{},
	}
}
