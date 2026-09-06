package log

import (
	"os"
	"strings"
	"testing"
)

func TestPrintLnFormatsSeverityTagAndMessage(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "log-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = output
	t.Cleanup(func() {
		os.Stdout = original
		output.Close()
	})
	service := InitService()
	if service == nil {
		t.Fatal("InitService returned nil")
	}
	var want strings.Builder
	for _, severity := range []string{Error, Success, Warning, Info} {
		service.PrintLn(severity, "TEST", "a message with spaces")
		want.WriteString(severity + "[TEST]" + Reset + " a message with spaces\n")
	}
	got, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want.String() {
		t.Fatalf("unexpected log format: got %q, want %q", got, want.String())
	}
}
