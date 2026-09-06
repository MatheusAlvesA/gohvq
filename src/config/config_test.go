package config

import (
	"MatheusAlvesA/gohvq/src/log"
	"MatheusAlvesA/gohvq/src/repository"
	"MatheusAlvesA/gohvq/src/server"
	"os"
	"testing"
)

func TestDefaultConfigAppliedToServices(t *testing.T) {
	for _, configFile := range []struct {
		name  string
		data  string
		write bool
	}{
		{name: "missing"},
		{name: "empty", write: true},
		{name: "invalid JSON", data: "not JSON", write: true},
		{name: "empty object", data: "{}", write: true},
	} {
		t.Run(configFile.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if configFile.write {
				if err := os.WriteFile("gohvq_config.json", []byte(configFile.data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			c := InitService()
			token := c.AdminToken
			if !repository.IsValidKey(token) {
				t.Fatal("default admin token is not a valid generated key")
			}
			srv, repo, pst := server.InitServer(), repository.InitRepository(), repository.InitPersistence()
			c.SetServices(srv, repo, log.InitService(), pst)
			c.Start()
			if srv.Server.Addr != "0.0.0.0:4242" || !srv.CheckAdminToken(token) {
				t.Fatal("default address or generated admin token was not applied")
			}
			if repo.ClearMaxTime != 1 || repo.PingTimeout != 60 || repo.ClearFrequency != 10 {
				t.Fatal("default cleanup settings were not applied")
			}
			if !pst.Enabled || repo.Persistence != pst {
				t.Fatal("default persistence was not enabled and connected to the repository")
			}
		})
	}
}

func TestCustomConfigAppliedToServices(t *testing.T) {
	t.Chdir(t.TempDir())
	data := `{
		"localHostOnly": true,
		"serverPort": 8181,
		"adminToken": "custom_admin_token",
		"persistenceEnabled": false,
		"clearMaxSeconds": 3,
		"pingTimeout": 120,
		"clearFrequency": 20
	}`
	if err := os.WriteFile("gohvq_config.json", []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c := InitService()
	srv, repo, pst := server.InitServer(), repository.InitRepository(), repository.InitPersistence()
	c.SetServices(srv, repo, log.InitService(), pst)
	c.Start()
	if srv.Server.Addr != "127.0.0.1:8181" || !srv.CheckAdminToken("custom_admin_token") {
		t.Fatal("custom listen address or admin token was not applied")
	}
	if repo.ClearMaxTime != 3 || repo.PingTimeout != 120 || repo.ClearFrequency != 20 {
		t.Fatal("custom cleanup settings were not applied")
	}
	if pst.Enabled || repo.Persistence != pst {
		t.Fatal("disabled persistence setting or persistence wiring was not applied")
	}
}

func TestPartialConfigPreservesUnspecifiedDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("gohvq_config.json", []byte(`{"serverPort": 9090}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := InitService()
	before := *c
	// Config can also be loaded without any attached services or logger.
	c.Start()
	before.ServerPort = 9090
	if *c != before {
		t.Fatalf("partial config changed unspecified fields: got %+v, want %+v", *c, before)
	}
}
