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
		{name: "invalid origin type", data: `{"corsAllowedOrigin":123}`, write: true},
		{name: "invalid after origin", data: `{"corsAllowedOrigin":"https://app.example.com","pingTimeout":"invalid"}`, write: true},
		{name: "invalid header type", data: `{"clientIPHeader":123}`, write: true},
		{name: "invalid after header", data: `{"clientIPHeader":"X-Real-IP","pingTimeout":"invalid"}`, write: true},
		{name: "negative IP limit", data: `{"maxEntriesPerIP":-1}`, write: true},
		{name: "invalid after IP limit", data: `{"maxEntriesPerIP":3,"pingTimeout":"invalid"}`, write: true},
		{name: "empty", write: true},
		{name: "invalid JSON", data: "not JSON", write: true},
		{name: "empty object", data: "{}", write: true},
		{name: "type error after persistence change", data: `{"persistenceEnabled":false,"serverPort":"invalid"}`, write: true},
		{name: "type error after token and address change", data: `{"adminToken":"replacement_token","localHostOnly":true,"serverPort":8181,"pingTimeout":"invalid"}`, write: true},
		{name: "syntax error after valid fields", data: `{"persistenceEnabled":false,"serverPort":8181,`, write: true},
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
			if srv.CORSAllowedOrigin != "" || c.CORSAllowedOrigin != "" {
				t.Fatal("default CORS origin was not preserved")
			}
			if srv.ClientIPHeader != "" {
				t.Fatal("default client IP header was not preserved")
			}
			if srv.Server.Addr != "0.0.0.0:4242" || !srv.CheckAdminToken(token) {
				t.Fatal("default address or generated admin token was not applied")
			}
			if repo.ClearMaxTime != 1 || repo.PingTimeout != 60 || repo.ClearFrequency != 10 || repo.MaxEntriesPerIP != 0 {
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
		"clientIPHeader": "X-Real-IP",
		"corsAllowedOrigin": "https://app.example.com",
		"persistenceEnabled": false,
		"clearMaxSeconds": 3,
		"pingTimeout": 120,
		"clearFrequency": 20,
 "maxEntriesPerIP": 3
	}`
	if err := os.WriteFile("gohvq_config.json", []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c := InitService()
	srv, repo, pst := server.InitServer(), repository.InitRepository(), repository.InitPersistence()
	c.SetServices(srv, repo, log.InitService(), pst)
	c.Start()
	if srv.CORSAllowedOrigin != "https://app.example.com" {
		t.Fatal("custom CORS origin was not applied")
	}
	if srv.ClientIPHeader != "X-Real-IP" {
		t.Fatal("custom client IP header was not applied")
	}
	if srv.Server.Addr != "127.0.0.1:8181" || !srv.CheckAdminToken("custom_admin_token") {
		t.Fatal("custom listen address or admin token was not applied")
	}
	if repo.ClearMaxTime != 3 || repo.PingTimeout != 120 || repo.ClearFrequency != 20 || repo.MaxEntriesPerIP != 3 {
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
