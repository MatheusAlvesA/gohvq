package repository

import (
	"crypto/rand"
	"math/big"
	"sync"
	"time"
)

const KEY_SIZE uint = 64
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Repository struct {
	Head    *QueueHead
	ItemMap *map[string]*QueueItem
	mu      sync.RWMutex
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
	for selectedKey == "" || (*r.ItemMap)[selectedKey] != nil {
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
	(*r.ItemMap)[selectedKey] = newItem
	return newItem, nil
}

func (r *Repository) GetAndPingItemByKey(key string) (*QueueItem, uint64) {
	r.mu.RLock()
	item := (*r.ItemMap)[key]
	r.mu.RUnlock()
	if item == nil {
		return nil, 0
	}

	item.LastPing.Store(time.Now().Unix())
	deleted := r.Head.Deleted.Load()
	var position uint64 = 0
	if deleted < item.EnterPos {
		position = item.EnterPos - deleted
	}

	return item, position
}

func InitRepository() *Repository {
	return &Repository{
		Head:    &QueueHead{},
		ItemMap: &map[string]*QueueItem{},
	}
}
