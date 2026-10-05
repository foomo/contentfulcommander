package commanderclient

import (
	"runtime"
	"testing"
)

// BenchmarkGetReferrers compares the index-backed GetReferrers with the brute-force scan
// it replaces, on a 100k-entity cache.
func BenchmarkGetReferrers(b *testing.B) {
	client := newBenchClient(b, benchCacheSize)
	client.GetReferrers("e0", ReferrerFilter{}) // build the index outside the timing
	for _, target := range []string{"e4242", "e0"} {
		b.Run(target+"/index", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				client.GetReferrers(target, ReferrerFilter{FieldNames: []string{"children"}})
			}
		})
		b.Run(target+"/scan", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				bruteReferrers(client, target, ReferrerFilter{FieldNames: []string{"children"}})
			}
		})
	}
}

// BenchmarkReferenceIndexBuild measures the first reverse query on a 100k-entity cache,
// which builds the index, and reports the retained index size.
func BenchmarkReferenceIndexBuild(b *testing.B) {
	client := newBenchClient(b, benchCacheSize)
	entity, _ := client.GetEntity("e4242")
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		client.refIndex = nil
		b.StartTimer()
		entity.GetParents(nil)
	}
	b.StopTimer()

	var before, after runtime.MemStats
	client.refIndex = nil
	runtime.GC()
	runtime.ReadMemStats(&before)
	entity.GetParents(nil)
	runtime.GC()
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/(1<<20), "index-MiB")
	b.ReportMetric(float64(int64(before.HeapAlloc))/(1<<20), "cache-MiB")
	runtime.KeepAlive(client)
}
