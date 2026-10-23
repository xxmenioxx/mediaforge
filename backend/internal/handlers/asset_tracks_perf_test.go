package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func BenchmarkAssetTracksPatchLargeOverrideSettings(b *testing.B) {
	const overrideCount = 4096
	const targetCount = 16

	db, router, root := newAssetTracksTestServer(b)
	if sqlDB, err := db.DB(); err != nil {
		b.Fatal(err)
	} else {
		defer sqlDB.Close()
	}

	overrides := make(map[string]AssetConversionOverrideState, overrideCount)
	paths := make([]string, targetCount)
	for index := 0; index < overrideCount; index++ {
		path := filepath.Join(root, fmt.Sprintf("asset-%04d.mkv", index))
		cleanPath := filepath.Clean(path)
		overrides[cleanPath] = AssetConversionOverrideState{
			KeepAudioStreams: []int{index % 4}, VideoCodec: "x265", QualityValue: 18,
		}
		if index < targetCount {
			paths[index] = path
			writeTestFile(b, path, "media")
		}
	}
	if err := saveAssetConversionOverrides(db, overrides); err != nil {
		b.Fatal(err)
	}
	encoded, err := json.Marshal(models.JSONMap{"entries": overrides})
	if err != nil {
		b.Fatal(err)
	}

	var sequence atomic.Uint64
	var latencyMu sync.Mutex
	latencies := make([]time.Duration, 0, b.N)
	failed := make(chan int, 1)
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			path := paths[int(sequence.Add(1)-1)%len(paths)]
			request := httptest.NewRequest(http.MethodPatch, "/api/assets/tracks?path="+url.QueryEscape(path), bytes.NewBufferString(`{"keepAudioStreams":[1]}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			started := time.Now()
			router.ServeHTTP(response, request)
			elapsed := time.Since(started)
			if response.Code != http.StatusOK {
				select {
				case failed <- response.Code:
				default:
				}
			}
			latencyMu.Lock()
			latencies = append(latencies, elapsed)
			latencyMu.Unlock()
		}
	})
	b.StopTimer()
	select {
	case status := <-failed:
		b.Fatalf("Save Tracks returned HTTP %d", status)
	default:
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	if len(latencies) > 0 {
		p50Index := (len(latencies) - 1) / 2
		p95Index := (len(latencies)*95+99)/100 - 1
		b.ReportMetric(float64(latencies[p50Index])/float64(time.Millisecond), "p50-ms/op")
		b.ReportMetric(float64(latencies[p95Index])/float64(time.Millisecond), "p95-ms/op")
	}
	b.ReportMetric(float64(len(encoded))/1024, "override-KiB")
}
