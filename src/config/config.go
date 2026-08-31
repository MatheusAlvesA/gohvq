package config

import (
	"MatheusAlvesA/gohvq/src/log"
	"MatheusAlvesA/gohvq/src/repository"
	"MatheusAlvesA/gohvq/src/server"
	"fmt"
)

type ConfigService struct {
	LocalhostOnly bool
	ServerPort    uint32
	AdminToken    string

	QueueClearMaxSeconds uint
	QueuePingTimeout     uint
	QueueClearFreq       uint

	srv *server.Server
	rep *repository.Repository
	log *log.LogService
}

func (c *ConfigService) applyInitialToServer() {
	if c.LocalhostOnly {
		c.srv.Server.Addr = fmt.Sprintf("127.0.0.1:%d", c.ServerPort)
	} else {
		c.srv.Server.Addr = fmt.Sprintf("0.0.0.0:%d", c.ServerPort)
	}
	c.srv.AccessTk = c.AdminToken
}

func (c *ConfigService) applyInitialToRepository() {
	c.rep.ClearFrequency = c.QueueClearFreq
	c.rep.ClearMaxTime = c.QueueClearMaxSeconds
	c.rep.PingTimeout = c.QueuePingTimeout
}

func (c *ConfigService) SetServices(srv *server.Server, repo *repository.Repository, log *log.LogService) {
	c.srv = srv
	c.rep = repo
	c.log = log
}

func (c *ConfigService) Start() {
	if c.srv != nil {
		c.applyInitialToServer()
	}
	if c.rep != nil {
		c.applyInitialToRepository()
	}
	c.Log(log.Warning, "Generated admin token: "+c.AdminToken)
}

func (r *ConfigService) Log(logType string, message string) {
	if r.log == nil {
		return
	}
	r.log.PrintLn(logType, "CONFIG", message)
}

func InitService() *ConfigService {
	key, err := repository.GenerateRandomKey()
	if err != nil {
		panic(err)
	}

	return &ConfigService{
		LocalhostOnly: false,
		ServerPort:    4242,
		AdminToken:    key,

		QueueClearMaxSeconds: 1,
		QueuePingTimeout:     60,
		QueueClearFreq:       10,
	}
}
