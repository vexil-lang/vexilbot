// internal/dashboard/handlers_storage.go
package dashboard

import (
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type storageRow struct {
	File       string
	Size       string
	Records    int
	SchemaHash string
	Oldest     string
	Newest     string
	Error      string
}

type storagePageData struct {
	Tab  string
	Rows []storageRow
}

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	matches, err := filepath.Glob(filepath.Join(s.deps.DataDir, "*.vxb"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var rows []storageRow
	for _, path := range matches {
		rows = append(rows, inspectVxb(path))
	}
	s.render(w, "storage", storagePageData{Tab: "storage", Rows: rows})
}

func inspectVxb(path string) storageRow {
	row := storageRow{File: filepath.Base(path)}
	f, err := os.Open(path)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	defer f.Close()

	info, _ := f.Stat()
	row.Size = humanSize(info.Size())

	// Read magic (4 bytes) + schema hash (32 bytes) = 36 bytes header
	var hdr [36]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		row.Error = "bad header"
		return row
	}
	// Show first 4 bytes of hash as 8 hex chars
	row.SchemaHash = fmt.Sprintf("%x", hdr[4:8])

	// Count records and track timestamps
	// Each record: [4 bytes LE length][N bytes payload]
	// The payload's first 8 bytes are a u64 nanosecond timestamp (best-effort heuristic
	// that works for logentry, webhookevent, scheduledrelease since all have ts/created_at first)
	var count int
	var oldest, newest uint64
	var lenBuf [4]byte
	for {
		_, err := io.ReadFull(f, lenBuf[:])
		if err != nil {
			break
		}
		n := binary.LittleEndian.Uint32(lenBuf[:])
		payload := make([]byte, n)
		if _, err := io.ReadFull(f, payload); err != nil {
			break
		}
		count++
		if len(payload) >= 8 {
			// vexil BitWriter writes u64 as 8 bytes little-endian
			ts := binary.LittleEndian.Uint64(payload[:8])
			if ts > 0 {
				if oldest == 0 || ts < oldest {
					oldest = ts
				}
				if ts > newest {
					newest = ts
				}
			}
		}
	}
	row.Records = count
	if oldest > 0 {
		row.Oldest = time.Unix(0, int64(oldest)).UTC().Format("2006-01-02 15:04")
	}
	if newest > 0 {
		row.Newest = time.Unix(0, int64(newest)).UTC().Format("2006-01-02 15:04")
	}
	return row
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	}
}
