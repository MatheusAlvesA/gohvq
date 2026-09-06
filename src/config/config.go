package config

import (
	"MatheusAlvesA/gohvq/src/log"
	"MatheusAlvesA/gohvq/src/repository"
	"MatheusAlvesA/gohvq/src/server"
	"encoding/json/v2"
	"fmt"
	"os"
)

type ConfigService struct {
	LocalhostOnly bool   `json:"localHostOnly"`
	ServerPort    uint32 `json:"serverPort"`
	AdminToken    string `json:"adminToken"`

	PersistenceEnabled bool `json:"persistenceEnabled,omitempty"`

	QueueClearMaxSeconds uint `json:"clearMaxSeconds,omitempty"`
	QueuePingTimeout     uint `json:"pingTimeout,omitempty"`
	QueueClearFreq       uint `json:"clearFrequency,omitempty"`

	srv *server.Server          `json:"-"`
	rep *repository.Repository  `json:"-"`
	log *log.LogService         `json:"-"`
	pst *repository.Persistence `json:"-"`
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

func (c *ConfigService) applyInitialToPersistence() {
	c.pst.Enabled = c.PersistenceEnabled
}

func (c *ConfigService) SetServices(
	srv *server.Server,
	repo *repository.Repository,
	log *log.LogService,
	pst *repository.Persistence,
) {
	c.srv = srv
	c.rep = repo
	c.log = log
	c.pst = pst
}

func (c *ConfigService) Start() {
	c.loadConfig()
	if c.srv != nil {
		c.applyInitialToServer()
	}
	if c.rep != nil {
		c.applyInitialToRepository()
	}
	if c.pst != nil {
		c.applyInitialToPersistence()
	}
}

func (r *ConfigService) Log(logType string, message string) {
	if r.log == nil {
		return
	}
	r.log.PrintLn(logType, "CONFIG", message)
}

func (r *ConfigService) readConfigFile() []byte {
	fileBytes, err := os.ReadFile("gohvq_config.json")
	if err != nil {
		r.Log(log.Error, fmt.Sprintf("Failed to read config file: %s", err))
		return []byte{}
	}
	return fileBytes
}
func (c *ConfigService) loadConfig() {
	bytes := c.readConfigFile()
	if len(bytes) <= 1 {
		c.Log(log.Warning, "Using default config")
		c.Log(log.Warning, "Generated admin token: "+c.AdminToken)
		return
	}

	oldTk := c.AdminToken
	err := json.Unmarshal(bytes, c)
	if err != nil {
		c.Log(log.Error, fmt.Sprintf("Failed to unmarshal config JSON: %s", err))
		c.Log(log.Warning, "Using default config")
	}
	if oldTk == c.AdminToken {
		c.Log(log.Warning, "Generated admin token: "+c.AdminToken)
	}
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

		PersistenceEnabled: true,

		QueueClearMaxSeconds: 1,
		QueuePingTimeout:     60,
		QueueClearFreq:       10,
	}
}
