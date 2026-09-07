package repository

import (
	"MatheusAlvesA/gohvq/src/log"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const PERSISTENCE_ACTIONS_BUFFER_SIZE = 10_000
const PERSISTENCE_DB_FILE = "gohvq_persistence.db"
const PERSISTENCE_IP_SIZE = 45
const PERSISTANCE_DB_LINE_SIZE uint64 = uint64(KEY_SIZE) + 4 + PERSISTENCE_IP_SIZE // status + space + key + space + padded IP + newline

type Persistence struct {
	ItemMap map[string]*PersistenceItem
	// Enabled is configuration, set before Start. Runtime availability is kept
	// separately so producers never race with a worker disabling persistence.
	Enabled bool

	actions  chan *PersistanceAction
	stop     chan struct{}
	done     chan struct{}
	active   atomic.Bool
	stopOnce sync.Once

	counter uint64
	dbFile  *os.File
	log     *log.LogService
	wg      sync.WaitGroup
	mutex   sync.Mutex
}

func (p *Persistence) Log(logType string, message string) {
	if p.log != nil {
		p.log.PrintLn(logType, "PERSISTENCE", message)
	}
}

func (p *Persistence) SetLogService(logService *log.LogService) {
	p.log = logService
}

func (p *Persistence) addToFile(key, ip string) error {
	if !IsValidKey(key) || !validPersistenceIP(ip) {
		return errors.New("invalid persistence key or IP")
	}
	_, err := p.dbFile.Write([]byte("A " + key + " " + ip + strings.Repeat(" ", PERSISTENCE_IP_SIZE-len(ip)) + "\n"))
	return err
}

func (p *Persistence) removeFromFile(key string) error {
	item := p.ItemMap[key]
	if item == nil {
		return nil
	}
	_, err := p.dbFile.WriteAt([]byte("X"), int64(item.Index*PERSISTANCE_DB_LINE_SIZE))
	return err
}

func (p *Persistence) finishFromFile(key string) error {
	item := p.ItemMap[key]
	if item == nil || item.Finished {
		return nil
	}
	_, err := p.dbFile.WriteAt([]byte("F"), int64(item.Index*PERSISTANCE_DB_LINE_SIZE))
	return err
}

func (p *Persistence) optimizeDataBase(repo *Repository) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.dbFile == nil {
		return errors.New("persistence file is not open")
	}
	optimizedFile, err := os.Create(PERSISTENCE_DB_FILE + ".tmp")
	if err != nil {
		return fmt.Errorf("create optimized persistence file: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			optimizedFile.Close()
		}
	}()

	newItemMap := make(map[string]*PersistenceItem)
	var restored []*PersistenceItem
	var newCounter uint64
	line := make([]byte, PERSISTANCE_DB_LINE_SIZE)
	// ReadAt via SectionReader leaves the append offset unchanged on failure.
	reader := io.NewSectionReader(p.dbFile, 0, math.MaxInt64)
	for {
		_, err := io.ReadFull(reader, line)
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			p.Log(log.Warning, "Discarding incomplete final persistence record")
			break
		}
		if err != nil {
			return fmt.Errorf("read persistence record: %w", err)
		}
		key := string(line[2 : 2+int(KEY_SIZE)])
		ip := strings.TrimRight(string(line[3+int(KEY_SIZE):len(line)-1]), " ")
		if (line[0] != 'A' && line[0] != 'F' && line[0] != 'X') ||
			line[1] != ' ' || line[2+int(KEY_SIZE)] != ' ' || line[len(line)-1] != '\n' || !IsValidKey(key) || !validPersistenceIP(ip) {
			return errors.New("invalid persistence record")
		}
		if line[0] == 'X' {
			continue
		}
		if _, err := optimizedFile.Write(line); err != nil {
			return fmt.Errorf("write optimized persistence record: %w", err)
		}
		item := &PersistenceItem{Key: key, IP: ip, Index: newCounter, Finished: line[0] == 'F'}
		newItemMap[key] = item
		if repo != nil {
			restored = append(restored, item)
		}
		newCounter++
	}

	// Keep the original file and index together until replacement succeeds.
	// The replacement descriptor is already open and positioned for appends.
	if err := os.Rename(PERSISTENCE_DB_FILE+".tmp", PERSISTENCE_DB_FILE); err != nil {
		return fmt.Errorf("replace persistence file: %w", err)
	}
	p.dbFile.Close()
	p.dbFile = optimizedFile
	p.ItemMap = newItemMap
	p.counter = newCounter
	committed = true

	// Publish recovered queue nodes only after the entire file is accepted.
	for _, item := range restored {
		if item.Finished {
			repo.RegenerateFinishedItem(item.Key, item.IP)
		} else {
			repo.RegenerateQueueItem(item.Key, item.IP)
		}
	}
	p.Log(log.Info, "Persistence database optimized")
	return nil
}

func (p *Persistence) applyAction(action *PersistanceAction) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.dbFile == nil {
		return errors.New("persistence file is not open")
	}
	switch action.ActionType {
	case PersistenceAdd:
		if err := p.addToFile(action.Key, action.IP); err != nil {
			return err
		}
		p.ItemMap[action.Key] = &PersistenceItem{Key: action.Key, IP: action.IP, Index: p.counter}
		p.counter++
	case PersistenceRemove:
		if err := p.removeFromFile(action.Key); err != nil {
			return err
		}
		delete(p.ItemMap, action.Key)
	case PersistanceFinish:
		if err := p.finishFromFile(action.Key); err != nil {
			return err
		}
		if item := p.ItemMap[action.Key]; item != nil {
			item.Finished = true
		}
	case PersistenceClearQueue, PersistenceClearFinished:
		finished := action.ActionType == PersistenceClearFinished
		for key, item := range p.ItemMap {
			if item.Finished == finished {
				if err := p.removeFromFile(key); err != nil {
					return err
				}
				delete(p.ItemMap, key)
			}
		}
	default:
		return errors.New("unknown persistence action type")
	}
	return nil
}

func (p *Persistence) actionsConsumer() {
	defer p.wg.Done()
	defer func() {
		p.active.Store(false)
		close(p.done) // Release producers waiting on a full channel after failure.
	}()
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	consume := func(action *PersistanceAction) bool {
		if err := p.applyAction(action); err != nil {
			p.Log(log.Error, "Disabling persistence after write failure: "+err.Error())
			return false
		}
		return true
	}
	for {
		select {
		case action := <-p.actions:
			if !consume(action) {
				return
			}
		case <-ticker.C:
			if err := p.optimizeDataBase(nil); err != nil {
				p.Log(log.Error, "Could not optimize persistence: "+err.Error())
			}
		case <-p.stop:
			// Stop is called after request/cleanup producers have shut down.
			// Drain accepted actions before closing the file.
			for {
				select {
				case action := <-p.actions:
					if !consume(action) {
						return
					}
				default:
					return
				}
			}
		}
	}
}

// Start initializes persistence once, before starting repository workers or HTTP.
func (p *Persistence) Start(repo *Repository) {
	if !p.Enabled {
		return
	}
	dbFile, err := os.OpenFile(PERSISTENCE_DB_FILE, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		p.Log(log.Error, "Could not open persistence file: "+err.Error())
		return
	}
	p.dbFile = dbFile
	if err := p.Regenerate(repo); err != nil {
		p.Log(log.Error, "Could not restore persistence: "+err.Error())
		p.dbFile.Close()
		p.dbFile = nil
		return
	}
	p.wg.Add(1)
	p.active.Store(true)
	go p.actionsConsumer()
	p.Log(log.Info, "Started")
}

func (p *Persistence) Stop() {
	p.active.Store(false)
	if p.stop != nil {
		p.stopOnce.Do(func() { close(p.stop) })
	}
	p.wg.Wait()
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.dbFile != nil {
		p.dbFile.Close()
		p.dbFile = nil
	}
	p.Log(log.Info, "Stopped")
}

func (p *Persistence) enqueue(action *PersistanceAction, wait bool) {
	if !p.active.Load() {
		return
	}
	if wait {
		select {
		case p.actions <- action:
		case <-p.done:
		case <-p.stop:
		}
		return
	}
	select {
	case p.actions <- action:
	default:
		p.Log(log.Warning, "Persistence action channel is full, dropping action")
	}
}

func (p *Persistence) ClearAllNotFinished() {
	p.enqueue(&PersistanceAction{ActionType: PersistenceClearQueue}, true)
}

func (p *Persistence) ClearAllFinished() {
	p.enqueue(&PersistanceAction{ActionType: PersistenceClearFinished}, true)
}

func (p *Persistence) AddItem(key, ip string) {
	p.enqueue(&PersistanceAction{ActionType: PersistenceAdd, Key: key, IP: ip}, false)
}

func (p *Persistence) RemoveItem(key string) {
	p.enqueue(&PersistanceAction{ActionType: PersistenceRemove, Key: key}, false)
}

func (p *Persistence) FinishItem(key string) {
	p.enqueue(&PersistanceAction{ActionType: PersistanceFinish, Key: key}, true)
}

func (p *Persistence) Regenerate(repo *Repository) error {
	if !p.Enabled {
		return nil
	}
	return p.optimizeDataBase(repo)
}

func InitPersistence() *Persistence {
	return &Persistence{
		ItemMap: make(map[string]*PersistenceItem),
		Enabled: true,
		actions: make(chan *PersistanceAction, PERSISTENCE_ACTIONS_BUFFER_SIZE),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Empty IPs represent tickets created without an address (for example imported records).
func validPersistenceIP(ip string) bool {
	if ip == "" {
		return true
	}
	addr, err := netip.ParseAddr(ip)
	return err == nil && addr.Zone() == "" && len(ip) <= PERSISTENCE_IP_SIZE
}
