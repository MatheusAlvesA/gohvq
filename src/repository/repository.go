package repository

import (
	"MatheusAlvesA/gohvq/src/log"
	"crypto/rand"
	"math/big"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

// KEY_SIZE defines the key length used by generation, validation, and persistence.
const KEY_SIZE uint = 20
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
	stopSignal atomic.Bool
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

func (r *Repository) CreateItem(ip string) (*ItemSnapshot, error) {
	if ip != "" {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return nil, err
		}
		ip = addr.WithZone("").Unmap().String()
	}
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
	newItem.IP = ip
	newItem.CreatedAt = time.Now().Unix()
	newItem.LastPing.Store(time.Now().Unix())
	r.Head.AddItem(newItem)
	r.ItemMap[selectedKey] = newItem

	r.Persistence.AddItem(selectedKey, ip)
	return newItem.snapshot(), nil
}

func (r *Repository) RegenerateQueueItem(key, ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	newItem := new(QueueItem)
	newItem.Key = key
	newItem.IP = ip
	newItem.CreatedAt = time.Now().Unix()
	newItem.LastPing.Store(time.Now().Unix())
	r.Head.AddItem(newItem)
	r.ItemMap[key] = newItem
}
func (r *Repository) RegenerateFinishedItem(key, ip string) {
	r.muFinished.Lock()
	defer r.muFinished.Unlock()
	newItem := new(QueueItem)
	newItem.Key = key
	newItem.IP = ip
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
	// Keep the head address stable for lock-free size queries.
	r.Head.FirstItem = nil
	r.Head.LastItem = nil
	r.Head.Length.Store(0)
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

func (r *Repository) FinishItems(n uint) []*ItemSnapshot {
	if n == 0 {
		return make([]*ItemSnapshot, 0)
	}
	r.mu.Lock()
	r.muFinished.Lock()
	defer r.mu.Unlock()
	defer r.muFinished.Unlock()

	var resultList []*ItemSnapshot
	current := r.Head.PopItem()
	now := time.Now().Unix()
	for current != nil {
		current.Next = nil
		current.Previus = nil
		current.FinishedAt = now
		delete(r.ItemMap, current.Key)
		r.FinishedMap[current.Key] = current
		resultList = append(resultList, current.snapshot())
		r.Persistence.FinishItem(current.Key)
		n--
		if n <= 0 {
			break
		}
		current = r.Head.PopItem()
	}

	return resultList
}

func (r *Repository) GetFinished(key string) *ItemSnapshot {
	r.muFinished.RLock()
	defer r.muFinished.RUnlock()

	return r.FinishedMap[key].snapshot()
}

func (r *Repository) DeleteFinished(key string) *ItemSnapshot {
	r.muFinished.Lock()
	defer r.muFinished.Unlock()

	item := r.FinishedMap[key]
	if item != nil {
		delete(r.FinishedMap, key)
		r.Persistence.RemoveItem(key)
	}
	return item.snapshot()
}

func (r *Repository) GetAndPingItemByKey(key string) *ItemSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item := r.ItemMap[key]
	if item == nil {
		return nil
	}

	item.LastPing.Store(time.Now().Unix())
	return item.snapshot()
}

func (r *Repository) clearTask() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	defer r.wg.Done()

	for true {
		<-ticker.C
		if r.stopSignal.Load() {
			return
		}
		r.clearIfDue()
	}
}

func (r *Repository) clearIfDue() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Now().Unix()-r.lastClear >= int64(r.ClearFrequency) {
		r.doClearLocked()
	}
}

func (r *Repository) doClear() cleanupStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.doClearLocked()
}

// cleanupStats counts completed work, excluding the ticket that observes timeout.
type cleanupStats struct {
	processed uint64
	removed   uint64
	timedOut  bool
}

func (r *Repository) doClearLocked() (stats cleanupStats) {
	// Even a scan that exhausts its time budget must respect ClearFrequency.
	initialSize := r.Head.Length.Load()
	var currentPosition uint64
	defer func() {
		r.lastClear = time.Now().Unix()
		stats.removed = initialSize - r.Head.Length.Load()
		stats.processed = currentPosition + stats.removed
	}()
	now := time.Now().Unix()

	currentItem := r.Head.FirstItem
	timeout := time.NewTimer(time.Duration(r.ClearMaxTime) * time.Second)
	defer timeout.Stop()
	for currentItem != nil {
		currentItem.Position = currentPosition
		select {
		case <-timeout.C:
			stats.timedOut = true
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
	return
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
	r.stopSignal.Store(false)
	r.wg.Add(1)
	go r.clearTask()
	r.Log(log.Info, "Started")
}
func (r *Repository) Stop() {
	r.stopSignal.Store(true)
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
