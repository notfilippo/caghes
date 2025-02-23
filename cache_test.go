// Copyright 2025 Filippo Rossi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otto_test

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"path"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/notfilippo/otto"
)

const ChunkSize = 1 << 16
const ChunkCount = 1 << 5

func TestSaveAndLoad(t *testing.T) {
	savePath := path.Join(t.TempDir(), "cache")

	expected := []byte{1, 2, 3}

	cache := otto.New(1<<10, 10)
	cache.Set("example", expected)
	err := cache.SaveToFile(savePath)
	if err != nil {
		t.Errorf("failed to save cache to file: %v", err)
	}

	loadedCache, err := otto.LoadFromFile(savePath)
	if err != nil {
		t.Errorf("failed to load cache from file: %v", err)
	}

	actual := loadedCache.Get("example", nil)

	if !bytes.Equal(actual, expected) {
		t.Errorf("serialized cache file is corrupted")
	}
}

func TestWriteToBuffer(t *testing.T) {
	cache := otto.New(ChunkSize, ChunkCount)

	value := [(ChunkSize * 3) / 2]byte{0xAA}

	cache.Set("example", value[:])

	allocatedByOtto := cache.Get("example", nil)
	if !bytes.Equal(allocatedByOtto, value[:]) {
		t.Fatalf("cached value should not be corrupted %v != %v (expected)", allocatedByOtto, value)
	}

	allocatedByTest := make([]byte, len(value))
	cache.Get("example", allocatedByTest)
	if !bytes.Equal(allocatedByTest, value[:]) {
		t.Fatalf("cached value should not be corrupted %v != %v (expected)", allocatedByOtto, value)
	}
}

func TestConcurrent(t *testing.T) {
	r := rand.NewChaCha8([32]byte{0})
	ri := rand.New(rand.NewPCG(1, 2))

	otto.Debug = true
	cache := otto.New(ChunkSize, ChunkCount)

	var wg sync.WaitGroup
	for key := range 10_000 {
		wg.Add(1)

		go func(key string) {
			defer wg.Done()

			size := ri.IntN(ChunkSize*ChunkCount-1) + 1
			value := make([]byte, size)
			_, _ = r.Read(value)

			cache.Set(key, value)

			for range 200 {
				wg.Add(1)

				go func() {
					defer wg.Done()

					result := cache.Get(key, nil)
					if bytes.Equal(result, value) {
						return
					} else {
						return
					}
				}()
			}

		}(strconv.Itoa(key))
	}

	wg.Wait()
}

type GenericCache interface {
	Set(key string, buf []byte)
	Get(key string, buf []byte) []byte
}

type FastCacheAdapter struct {
	inner *fastcache.Cache
}

func (c *FastCacheAdapter) Set(key string, buf []byte) {
	c.inner.Set([]byte(key), buf)
}

func (c *FastCacheAdapter) Get(key string, buf []byte) []byte {
	return c.inner.Get(buf, []byte(key))
}

func NewFastCache() GenericCache {
	return &FastCacheAdapter{fastcache.New(ChunkSize * ChunkCount)}
}

func NewOttoCache() GenericCache {
	return otto.New(ChunkSize, ChunkCount)
}

func random10k(tb testing.TB, cache GenericCache) {
	r := rand.NewChaCha8([32]byte{0})
	ri := rand.New(rand.NewPCG(1, 2))

	for j := range 10_000 {
		for i := range 10 {
			size := ri.IntN((1<<i)*10-1) + 1

			buf := make([]byte, size)
			_, err := r.Read(buf)
			if err != nil {
				tb.Fatalf("unexpected error from random source: %v", err)
			}

			key := fmt.Sprintf("%d_%d", j, i)
			cache.Set(key, buf)

			res := cache.Get(key, nil)

			if !bytes.Equal(res, buf) {
				tb.Fatalf("cached value for %s should not be corrupted %v != %v (expected)", key, res, buf)
			}
		}
	}
}

func TestCacheRandom10k(t *testing.T) {
	otto.Debug = true
	random10k(t, NewOttoCache())
}

func BenchmarkFastCacheRandom10k(b *testing.B) {
	b.ReportAllocs()
	cache := NewFastCache()
	runtime.GC()
	b.ResetTimer()
	for range b.N {
		random10k(b, cache)
	}
}

func BenchmarkCacheRandom10k(b *testing.B) {
	b.ReportAllocs()
	cache := NewOttoCache()
	runtime.GC()
	b.ResetTimer()
	for range b.N {
		random10k(b, cache)
	}
}
