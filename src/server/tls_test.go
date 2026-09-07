package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testTLSFiles(t *testing.T) (string, string, *x509.CertPool) {
	t.Helper()
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	defer fixture.Close()
	pair := fixture.TLS.Certificates[0]
	key, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(fixture.Certificate())
	return certFile, keyFile, roots
}

func TestServerHTTPSOnly(t *testing.T) {
	cert, key, roots := testTLSFiles(t)
	s := newTestServer()
	s.TLSCertFile, s.TLSKeyFile = cert, key
	s.Server.Addr = "127.0.0.1:0"
	listening := make(chan string, 1)
	s.Server.BaseContext = func(l net.Listener) context.Context {
		listening <- l.Addr().String()
		return context.Background()
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	var addr string
	select {
	case addr = <-listening:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	res, err := client.Post("https://"+addr+"/enter", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || res.TLS == nil || s.repo.GetCurrentQueueSize() != 1 {
		t.Fatal("HTTPS did not create a ticket")
	}
	res, err = client.Post("http://"+addr+"/enter", "", nil)
	if err == nil {
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("plaintext request returned %d", res.StatusCode)
		}
	}
	if s.repo.GetCurrentQueueSize() != 1 {
		t.Fatal("plaintext request changed the queue")
	}
	s.Stop()
	res, err = client.Post("https://"+addr+"/enter", "", nil)
	if err == nil {
		res.Body.Close()
		t.Fatal("HTTPS accepted a request after shutdown")
	}
}

func TestServerRejectsInvalidTLSConfiguration(t *testing.T) {
	cert, key, _ := testTLSFiles(t)
	otherCert, _, _ := testTLSFiles(t)
	// Malformed PEM must fail before opening a listener.
	if err := os.WriteFile(otherCert, []byte("invalid certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, cert, key string }{
		{"certificate only", cert, ""}, {"key only", "", key},
		{"missing certificate", cert + ".missing", key}, {"missing key", cert, key + ".missing"},
		{"malformed certificate", otherCert, key}, {"invalid private key", cert, cert},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer()
			s.TLSCertFile, s.TLSKeyFile = tc.cert, tc.key
			s.Server.Addr = "invalid address"
			err := s.Start()
			if err == nil {
				s.Stop()
				t.Fatal("invalid TLS configuration was accepted")
			}
			if s.Server.TLSConfig != nil {
				t.Fatal("invalid TLS configuration was applied")
			}
		})
	}
}
