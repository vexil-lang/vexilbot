package vexstore

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

// .vxb file format:
//   [4 bytes]  magic: 0x56 0x58 0x42 0x00  ("VXB\0")
//   [32 bytes] schema hash (from generated SchemaHash variable)
//   [records...]
//
// Each record:
//   [4 bytes LE] payload length N
//   [N bytes]    vexil-encoded payload

var magic = [4]byte{0x56, 0x58, 0x42, 0x00}

const headerSize = 4 + 32

// AppendStore is a single append-only .vxb file. Safe for concurrent Append calls.
type AppendStore struct {
	mu       sync.Mutex
	f        *os.File
	schemaID [32]byte
}

// OpenAppendStore opens or creates the .vxb file at path.
// Validates the header schema hash against schemaID if the file already exists.
// Pass the generated SchemaHash from the logentry or webhookevent package as schemaID.
func OpenAppendStore(path string, schemaID [32]byte) (*AppendStore, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("vexstore: open %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.Size() == 0 {
		if err := writeHeader(f, schemaID); err != nil {
			f.Close()
			return nil, err
		}
	} else {
		if err := validateHeader(f, schemaID); err != nil {
			f.Close()
			return nil, err
		}
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, err
	}
	return &AppendStore{f: f, schemaID: schemaID}, nil
}

// Append writes a single length-prefixed record. Safe for concurrent use.
func (s *AppendStore) Append(record []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(record)))
	if _, err := s.f.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("vexstore: write length: %w", err)
	}
	if _, err := s.f.Write(record); err != nil {
		return fmt.Errorf("vexstore: write record: %w", err)
	}
	return nil
}

// ReadAll reads all records from the beginning of the file (after the header).
func (s *AppendStore) ReadAll() ([][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.f.Seek(int64(headerSize), io.SeekStart); err != nil {
		return nil, err
	}
	var records [][]byte
	var lenBuf [4]byte
	for {
		_, err := io.ReadFull(s.f, lenBuf[:])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("vexstore: read length: %w", err)
		}
		n := binary.LittleEndian.Uint32(lenBuf[:])
		rec := make([]byte, n)
		if _, err := io.ReadFull(s.f, rec); err != nil {
			return nil, fmt.Errorf("vexstore: read record body (%d bytes): %w", n, err)
		}
		records = append(records, rec)
	}
	if _, err := s.f.Seek(0, io.SeekEnd); err != nil {
		return nil, err
	}
	return records, nil
}

// Close flushes and closes the underlying file.
func (s *AppendStore) Close() error { return s.f.Close() }

func writeHeader(f *os.File, schemaID [32]byte) error {
	if _, err := f.Write(magic[:]); err != nil {
		return fmt.Errorf("vexstore: write magic: %w", err)
	}
	if _, err := f.Write(schemaID[:]); err != nil {
		return fmt.Errorf("vexstore: write schema hash: %w", err)
	}
	return nil
}

func validateHeader(f *os.File, schemaID [32]byte) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var hdr [headerSize]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return fmt.Errorf("vexstore: read header: %w", err)
	}
	if hdr[0] != magic[0] || hdr[1] != magic[1] || hdr[2] != magic[2] || hdr[3] != magic[3] {
		return fmt.Errorf("vexstore: bad magic bytes: % x", hdr[:4])
	}
	var got [32]byte
	copy(got[:], hdr[4:36])
	if got != schemaID {
		return fmt.Errorf("vexstore: schema hash mismatch: file has %x, want %x", got, schemaID)
	}
	return nil
}
