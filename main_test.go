package main

import (
	"encoding/json"
	"net/http"
	"syscall"
	"testing"
	"time"
)

func TestMainStartStop(t *testing.T) {
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	// wait for the server to be up
	var res *http.Response
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err = http.Get("http://localhost:4242/position?key=invalid")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Server did not start: %q", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Incorrect status for invalid key, expected 400 got %d", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("Fail to parse response %q", err)
	}
	if body["message"] != "Invalid key" {
		t.Errorf("Incorrect message, got %q", body["message"])
	}

	// shutdown
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("Fail to send SIGINT: %q", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("main did not return after SIGINT")
	}
}
