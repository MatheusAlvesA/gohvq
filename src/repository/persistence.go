package repository

import (
	"MatheusAlvesA/gohvq/src/log"
	"sync"
)

type Persistence struct {
	ItemMap map[string]*QueueItem
	Enabled bool

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

func (p *Persistence) Start(repo *Repository) {
	p.wg.Add(1)
	p.wg.Done()
	p.Log(log.Info, "Started")
}
func (p *Persistence) Stop() {
	p.stopSignal = true
	p.wg.Wait()
	p.Log(log.Info, "Stopped")
}

func (p *Persistence) Regenerate(repo *Repository) {
	//TODO
}

func InitPersistence() *Persistence {
	return &Persistence{
		ItemMap: map[string]*QueueItem{},
		Enabled: true,
	}
}
