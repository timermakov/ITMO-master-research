package mmapstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	mmap "github.com/edsrzf/mmap-go"
)

const pageSizeBytes = 4096

// Store manages a memory-mapped file that can be used to simulate
// cold vs warm start behavior by touching pages and measuring access latency.
type Store struct {
	filePath  string
	sizeBytes int64

	mu   sync.Mutex
	file *os.File
	data mmap.MMap
}

// New creates a new Store with provided file path and target size in bytes.
func New(filePath string, sizeBytes int64) *Store {
	return &Store{filePath: filePath, sizeBytes: sizeBytes}
}

// EnsureFile creates the backing file and fills it with a simple pattern so
// the OS actually allocates pages on first read.
func (s *Store) EnsureFile() (err error) {
	if s.sizeBytes <= 0 {
		return errors.New("sizeBytes must be > 0")
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()

	if err := f.Truncate(s.sizeBytes); err != nil {
		return err
	}

	// Write a small pattern at the start of each page to avoid sparse file
	// optimization hiding real IO on some filesystems.
	buf := make([]byte, pageSizeBytes)
	for i := 0; i < len(buf); i += 8 {
		binary.LittleEndian.PutUint64(buf[i:], 0xdeadbeefcafebabe)
	}
	for off := int64(0); off < s.sizeBytes; off += pageSizeBytes {
		if _, err := f.WriteAt(buf, off); err != nil {
			return err
		}
	}
	return nil
}

// Map opens and memory-maps the file. Reentrant safe.
func (s *Store) Map() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data != nil {
		return nil
	}
	f, err := os.OpenFile(s.filePath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return closeErr
		}
		return err
	}
	if info.Size() == 0 {
		if closeErr := f.Close(); closeErr != nil {
			return closeErr
		}
		return fmt.Errorf("backing file is empty: %s", s.filePath)
	}
	mm, err := mmap.Map(f, mmap.RDWR, 0)
	if err != nil {
		if closeErr := f.Close(); closeErr != nil {
			return closeErr
		}
		return err
	}
	s.file = f
	s.data = mm
	return nil
}

// Remap unmaps and maps the backing file again (cold page faults on next read).
func (s *Store) Remap() error {
	if err := s.Unmap(); err != nil {
		return err
	}
	return s.Map()
}

// Unmap releases resources.
func (s *Store) Unmap() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err1, err2 error
	if s.data != nil {
		err1 = s.data.Unmap()
		s.data = nil
	}
	if s.file != nil {
		err2 = s.file.Close()
		s.file = nil
	}
	if err1 != nil {
		return err1
	}
	return err2
}

// TouchSequential accesses one byte per page to populate pages into memory.
func (s *Store) TouchSequential() error {
	s.mu.Lock()
	data := s.data
	s.mu.Unlock()
	if data == nil {
		return errors.New("store not mapped")
	}
	for off := 0; off < len(data); off += pageSizeBytes {
		_ = data[off]
	}
	return nil
}

// pageCount returns the number of pages in the mapped file.
func (s *Store) pageCount() int {
	s.mu.Lock()
	data := s.data
	s.mu.Unlock()
	if data == nil {
		return 0
	}
	return len(data) / pageSizeBytes
}

// RandomPageIndices returns a deterministic slice of random page indices
// for the given number of samples and RNG seed.
func (s *Store) RandomPageIndices(samples int, seed int64) []int {
	if samples <= 0 {
		return nil
	}
	pages := s.pageCount()
	if pages == 0 {
		return nil
	}
	r := rand.New(rand.NewSource(seed))
	indices := make([]int, samples)
	for i := 0; i < samples; i++ {
		indices[i] = r.Intn(pages)
	}
	return indices
}

// MeasureReadDurationAt performs reads at the provided page indices and
// returns total duration. It is deterministic for a fixed index slice.
func (s *Store) MeasureReadDurationAt(indices []int) time.Duration {
	s.mu.Lock()
	data := s.data
	s.mu.Unlock()
	if data == nil || len(indices) == 0 {
		return 0
	}
	start := time.Now()
	var sink uint64
	for _, pageIdx := range indices {
		if pageIdx < 0 {
			pageIdx = 0
		}
		off := pageIdx * pageSizeBytes
		if off >= len(data) {
			continue
		}
		sink += uint64(data[off])
	}
	runtime.KeepAlive(sink)
	return time.Since(start)
}

// MeasureReadLatenciesAt performs reads at the provided page indices and
// returns per-read durations. It is deterministic for a fixed index slice.
func (s *Store) MeasureReadLatenciesAt(indices []int) []time.Duration {
	s.mu.Lock()
	data := s.data
	s.mu.Unlock()
	if data == nil || len(indices) == 0 {
		return nil
	}
	latencies := make([]time.Duration, len(indices))
	var sink uint64
	for i, pageIdx := range indices {
		if pageIdx < 0 {
			pageIdx = 0
		}
		off := pageIdx * pageSizeBytes
		if off >= len(data) {
			continue
		}
		start := time.Now()
		sink += uint64(data[off])
		latencies[i] = time.Since(start)
	}
	runtime.KeepAlive(sink)
	return latencies
}

// MeasureRandomReadLatencies performs N random page reads and returns per-read durations.
func (s *Store) MeasureRandomReadLatencies(samples int) []time.Duration {
	s.mu.Lock()
	data := s.data
	s.mu.Unlock()
	if data == nil || samples <= 0 {
		return nil
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	pages := len(data) / pageSizeBytes
	if pages == 0 {
		return nil
	}
	latencies := make([]time.Duration, samples)
	var sink uint64
	for i := 0; i < samples; i++ {
		off := (r.Intn(pages)) * pageSizeBytes
		start := time.Now()
		sink += uint64(data[off])
		latencies[i] = time.Since(start)
	}
	runtime.KeepAlive(sink)
	return latencies
}

// MeasureRandomReads performs N random page reads and returns total duration.
func (s *Store) MeasureRandomReads(samples int) time.Duration {
	latencies := s.MeasureRandomReadLatencies(samples)
	var total time.Duration
	for _, d := range latencies {
		total += d
	}
	return total
}

// SizeMB returns mapped size in MB.
func (s *Store) SizeMB() int {
	return int(s.sizeBytes / (1024 * 1024))
}
