package repository

import (
	"MatheusAlvesA/gohvq/src/log"
	"os"
	"sync"
	"time"
)

const PERSISTENCE_ACTIONS_BUFFER_SIZE = 10_000
const PERSISTENCE_DB_FILE = "gohvq_persistence.db"
const PERSISTANCE_DB_LINE_SIZE uint64 = uint64(KEY_SIZE) + 3 // "A " + key + "\n"

type Persistence struct {
	ItemMap map[string]*PersistenceItem
	Enabled bool

	actions chan *PersistanceAction

	counter    uint64
	stopSignal bool
	dbFile     *os.File
	log        *log.LogService
	wg         sync.WaitGroup
}

func (p *Persistence) Log(logType string, message string) {
	if p.log == nil {
		return
	}
	p.log.PrintLn(logType, "PERSISTENCE", message)
}
func (p *Persistence) SetLogService(logService *log.LogService) {
	p.log = logService
}

func (p *Persistence) addToFile(key string) {
	if p.dbFile == nil {
		p.Log(log.Error, "Persistence file is not open")
		return
	}
	bytes := []byte("A " + key + "\n")
	_, err := p.dbFile.Write(bytes)
	if err != nil {
		p.Log(log.Error, "Could not append to persistence file: "+err.Error())
		p.Log(log.Error, "Closing db file...")
		p.dbFile.Close()
		p.dbFile = nil
	}
}
func (p *Persistence) removeFromFile(key string) {
	if p.dbFile == nil {
		p.Log(log.Error, "Persistence file is not open")
		return
	}
	if _, ok := p.ItemMap[key]; !ok {
		return
	}

	_, err := p.dbFile.WriteAt([]byte("X"), int64(p.ItemMap[key].Index*PERSISTANCE_DB_LINE_SIZE))
	if err != nil {
		p.Log(log.Error, "Could not edit persistence file: "+err.Error())
		p.Log(log.Error, "Closing db file...")
		p.dbFile.Close()
		p.dbFile = nil
	}
}
func (p *Persistence) finishFromFile(key string) {
	if p.dbFile == nil {
		p.Log(log.Error, "Persistence file is not open")
		return
	}
	if item, ok := p.ItemMap[key]; !ok || item.Finished {
		return
	}

	_, err := p.dbFile.WriteAt([]byte("F"), int64(p.ItemMap[key].Index*PERSISTANCE_DB_LINE_SIZE))
	if err != nil {
		p.Log(log.Error, "Could not edit persistence file: "+err.Error())
		p.Log(log.Error, "Closing db file...")
		p.dbFile.Close()
		p.dbFile = nil
	}
}

func (p *Persistence) actionsConsumer() {
	for {
		select {
		case action := <-p.actions:
			switch action.ActionType {
			case PersistenceAdd:
				p.ItemMap[action.Key] = &PersistenceItem{
					Key:      action.Key,
					Index:    p.counter,
					Finished: false,
				}
				p.counter++
				p.addToFile(action.Key)
			case PersistenceRemove:
				p.removeFromFile(action.Key)
				delete(p.ItemMap, action.Key)
			case PersistanceFinish:
				if item, ok := p.ItemMap[action.Key]; ok {
					item.Finished = true
				}
				p.finishFromFile(action.Key)
			default:
				p.Log(log.Error, "Unknown persistence action type")
			}
		default:
			if p.stopSignal {
				p.wg.Done()
				return
			}
			if p.dbFile == nil {
				p.wg.Done()
				p.Log(log.Error, "Persistence file is not open, stopping actions consumer and disabling persistence")
				p.Enabled = false
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (p *Persistence) Start(repo *Repository) {
	dbFile, fileError := os.OpenFile(PERSISTENCE_DB_FILE, os.O_CREATE|os.O_RDWR, 0644)
	if fileError != nil {
		p.Log(log.Error, "Could not open persistence file: "+fileError.Error())
		return
	} else {
		p.dbFile = dbFile
	}
	p.wg.Add(1)
	go p.actionsConsumer()
	p.Log(log.Info, "Started")
}
func (p *Persistence) Stop() {
	p.stopSignal = true
	p.wg.Wait()
	if p.dbFile != nil {
		p.dbFile.Close()
		p.dbFile = nil
	}
	p.Log(log.Info, "Stopped")
}

func (p *Persistence) ClearAllNotFinished() {
	for key, item := range p.ItemMap {
		if !item.Finished {
			p.actions <- &PersistanceAction{
				ActionType: PersistenceRemove,
				Key:        key,
			}
		}
	}
}
func (p *Persistence) ClearAllFinished() {
	for key, item := range p.ItemMap {
		if item.Finished {
			p.actions <- &PersistanceAction{
				ActionType: PersistenceRemove,
				Key:        key,
			}
		}
	}
}
func (p *Persistence) AddItem(key string) {
	if !p.Enabled {
		return
	}
	select {
	case p.actions <- &PersistanceAction{
		ActionType: PersistenceAdd,
		Key:        key,
	}:
	default:
		p.Log(log.Warning, "Persistence action channel is full, dropping add action")
	}
}
func (p *Persistence) RemoveItem(key string) {
	if !p.Enabled {
		return
	}
	select {
	case p.actions <- &PersistanceAction{
		ActionType: PersistenceRemove,
		Key:        key,
	}:
	default:
		p.Log(log.Warning, "Persistence action channel is full, dropping remove action")
	}
}
func (p *Persistence) FinishItem(key string) {
	if !p.Enabled {
		return
	}
	p.actions <- &PersistanceAction{
		ActionType: PersistanceFinish,
		Key:        key,
	}
}

func (p *Persistence) Regenerate(repo *Repository) {
	//TODO
}

func InitPersistence() *Persistence {
	return &Persistence{
		ItemMap: map[string]*PersistenceItem{},
		Enabled: true,

		actions: make(chan *PersistanceAction, PERSISTENCE_ACTIONS_BUFFER_SIZE),
	}
}
