package repository

import (
	"flag"
	"strconv"
	"strings"
	"testing"
	"time"
)

var cleanupTickets = flag.Int("cleanup-tickets", 1_000_000, "tickets prepared for each cleanup benchmark iteration")
var cleanupSeconds = flag.Uint("cleanup-seconds", 1, "cleanup time budget in whole seconds")

// BenchmarkCleanupCapacity measures real cleanup work, excluding queue creation.
// Use -benchtime=1x to avoid repeatedly rebuilding large queues during calibration.
func BenchmarkCleanupCapacity(b *testing.B) {
	if *cleanupTickets <= 0 || *cleanupSeconds == 0 {
		b.Fatal("cleanup-tickets and cleanup-seconds must be positive")
	}
	for _, expired := range []bool{false, true} {
		name := "active"
		if expired {
			name = "expired"
		}
		b.Run(name, func(b *testing.B) {
			var processed, removed, timeouts uint64
			var elapsed time.Duration
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				r := InitRepository()
				r.ClearMaxTime = *cleanupSeconds
				// Active tickets stay alive throughout a long benchmark run.
				r.PingTimeout = ^uint(0) >> 1
				ping := time.Now().Unix()
				if expired {
					r.PingTimeout = 60
					ping -= 61
				}
				r.ItemMap = make(map[string]*QueueItem, *cleanupTickets)
				for ticket := 0; ticket < *cleanupTickets; ticket++ {
					// Unique fixed-length keys without benchmarking crypto/rand.
					suffix := strconv.FormatUint(uint64(ticket), 36)
					if len(suffix) > int(KEY_SIZE) {
						b.Fatal("ticket count exceeds key space")
					}
					key := strings.Repeat("0", int(KEY_SIZE)-len(suffix)) + suffix
					item := &QueueItem{Key: key}
					item.LastPing.Store(ping)
					r.Head.AddItem(item)
					r.ItemMap[key] = item
				}
				b.StartTimer()
				start := time.Now()
				stats := r.doClear()
				elapsed += time.Since(start)
				b.StopTimer()
				processed += stats.processed
				removed += stats.removed
				if stats.timedOut {
					timeouts++
				}
			}
			b.ReportMetric(float64(processed)/float64(b.N), "tickets/op")
			b.ReportMetric(float64(processed)/elapsed.Seconds(), "tickets/s")
			b.ReportMetric(float64(removed)/float64(b.N), "removed/op")
			b.ReportMetric(float64(timeouts)/float64(b.N), "timeouts/op")
		})
	}
}

func TestCleanupStats(t *testing.T) {
	for _, budget := range []uint{0, 1} {
		t.Run(strconv.FormatUint(uint64(budget), 10), func(t *testing.T) {
			r := InitRepository()
			r.ClearMaxTime = budget
			for _, expired := range []bool{false, true, false, true} {
				item, err := r.CreateItem("")
				if err != nil {
					t.Fatal(err)
				}
				if expired {
					r.ItemMap[item.Key].LastPing.Store(time.Now().Unix() - 61)
				}
			}
			stats := r.doClear()
			want := cleanupStats{processed: 4, removed: 2}
			if budget == 0 {
				want = cleanupStats{timedOut: true}
			}
			if stats != want {
				t.Fatalf("got %+v, want %+v", stats, want)
			}
		})
	}
}
