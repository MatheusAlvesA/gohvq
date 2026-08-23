package repository

import (
	"crypto/rand"
	"math/big"
)

const KEY_SIZE uint = 64
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Repository struct {
	Head *QueueHead
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
	newKey, err := generateRandomKey()
	if err != nil {
		return nil, err
	}
	newItem := new(QueueItem)
	newItem.Key = newKey
	r.Head.addItem(newItem)
	return newItem, nil
}

func (r *Repository) GetAndPingItemByKey(key string) (*QueueItem, uint64) {
	currentItem := r.Head.firstItem
	var counter uint64 = 0
	for currentItem != nil {
		if currentItem.Key == key {
			return currentItem, counter
		}
		currentItem = currentItem.Next
		counter++
	}
	return nil, counter
}

func InitRepository() *Repository {
	return &Repository{
		Head: &QueueHead{},
	}
}
