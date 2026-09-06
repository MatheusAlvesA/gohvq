package repository

import (
	"MatheusAlvesA/gohvq/src/log"
	"sync"
	"time"
)

const PERSISTENCE_ACTIONS_BUFFER_SIZE = 10_000

type Persistence struct {
	ItemMap map[string]*PersistenceItem
	Enabled bool

	actions chan *PersistanceAction

	counter    uint64
	stopSignal bool
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
			case PersistenceRemove:
				delete(p.ItemMap, action.Key)
			case PersistanceFinish:
				if item, ok := p.ItemMap[action.Key]; ok {
					item.Finished = true
				}
			default:
				p.Log(log.Error, "Unknown persistence action type")
			}
		default:
			if p.stopSignal {
				p.wg.Done()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (p *Persistence) Start(repo *Repository) {
	p.wg.Add(1)
	go p.actionsConsumer()
	p.Log(log.Info, "Started")
}
func (p *Persistence) Stop() {
	p.stopSignal = true
	p.wg.Wait()
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
