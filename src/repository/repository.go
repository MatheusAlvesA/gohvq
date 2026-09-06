package repository

import (
	"MatheusAlvesA/gohvq/src/log"
	"crypto/rand"
	"math/big"
	"sync"
	"time"
)

const KEY_SIZE uint = 64
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Repository struct {
	Head        *QueueHead
	ItemMap     map[string]*QueueItem
	FinishedMap map[string]*QueueItem
	Persistence *Persistence

	PingTimeout    uint
	ClearFrequency uint
	ClearMaxTime   uint

	lastClear  int64
	stopSignal bool
	log        *log.LogService
	wg         sync.WaitGroup
	mu         sync.RWMutex
	muFinished sync.RWMutex
}

func GenerateRandomKey() (string, error) {
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
		newKey, err := GenerateRandomKey()
		if err != nil {
			return nil, err
		}
		selectedKey = newKey
	}

	newItem := new(QueueItem)
	newItem.Key = selectedKey
	newItem.CreatedAt = time.Now().Unix()
	newItem.LastPing.Store(time.Now().Unix())
	r.Head.AddItem(newItem)
	r.ItemMap[selectedKey] = newItem

	r.Persistence.AddItem(selectedKey)
	return newItem, nil
}

func (r *Repository) RegenerateQueueItem(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	newItem := new(QueueItem)
	newItem.Key = key
	newItem.CreatedAt = time.Now().Unix()
	newItem.LastPing.Store(time.Now().Unix())
	r.Head.AddItem(newItem)
	r.ItemMap[key] = newItem
}
func (r *Repository) RegenerateFinishedItem(key string) {
	r.muFinished.Lock()
	defer r.muFinished.Unlock()
	newItem := new(QueueItem)
	newItem.Key = key
	newItem.CreatedAt = time.Now().Unix()
	newItem.LastPing.Store(newItem.CreatedAt)
	newItem.FinishedAt = newItem.CreatedAt

	r.FinishedMap[key] = newItem
}

func (r *Repository) GetCurrentQueueSize() uint64 {
	return r.Head.Length.Load()
}

func (r *Repository) ClearQueue() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Head = &QueueHead{}
	r.ItemMap = map[string]*QueueItem{}
	r.Log(log.Warning, "Queue cleared")
	r.Persistence.ClearAllNotFinished()
}

func (r *Repository) ClearFinished() {
	r.muFinished.Lock()
	defer r.muFinished.Unlock()
	r.FinishedMap = map[string]*QueueItem{}
	r.Log(log.Warning, "Finished items cleared")
	r.Persistence.ClearAllFinished()
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
		r.Persistence.FinishItem(current.Key)
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
		r.Persistence.RemoveItem(key)
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
		if (now - r.lastClear) < int64(r.ClearFrequency) {
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
	timeout := time.NewTimer(time.Duration(r.ClearMaxTime) * time.Second)
	var currentPosition uint64 = 0
	for currentItem != nil {
		currentItem.Position = currentPosition
		select {
		case <-timeout.C:
			r.Log(log.Error, "Cloud not fully clean the queue in time")
			return
		default:
			elapsedTimeSeconds := now - currentItem.LastPing.Load()
			if elapsedTimeSeconds <= int64(r.PingTimeout) {
				currentItem = currentItem.Next
				currentPosition++
				continue
			}
			tmpNext := currentItem.Next
			r.Head.Detach(currentItem)
			delete(r.ItemMap, currentItem.Key)
			r.Persistence.RemoveItem(currentItem.Key)
			currentItem = tmpNext
		}
	}
}

func (r *Repository) Log(logType string, message string) {
	if r.log == nil {
		return
	}
	r.log.PrintLn(logType, "REPOSITORY", message)
}
func (s *Repository) SetLogService(logService *log.LogService) {
	s.log = logService
}

func (r *Repository) Start() {
	r.wg.Add(1)
	go r.clearTask()
	r.Log(log.Info, "Started")
}
func (r *Repository) Stop() {
	r.stopSignal = true
	r.wg.Wait()
	r.Log(log.Info, "Stopped")
}

func InitRepository() *Repository {
	return &Repository{
		Head:        &QueueHead{},
		ItemMap:     map[string]*QueueItem{},
		FinishedMap: map[string]*QueueItem{},

		Persistence: &Persistence{
			ItemMap: map[string]*PersistenceItem{},
			Enabled: false,
		},

		ClearFrequency: 10,
		PingTimeout:    60,
		ClearMaxTime:   1,
	}
}
