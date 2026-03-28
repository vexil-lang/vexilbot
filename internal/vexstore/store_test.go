package vexstore_test

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/vexstore"
)

func tempPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.vxb")
}

var testSchemaID = sha256.Sum256([]byte("test-schema-v1"))

func TestAppendStoreCreateAndRead(t *testing.T) {
	path := tempPath(t)
	s, err := vexstore.OpenAppendStore(path, testSchemaID)
	if err != nil {
		t.Fatalf("OpenAppendStore: %v", err)
	}
	record := []byte("hello world")
	if err := s.Append(record); err != nil {
		t.Fatalf("Append: %v", err)
	}
	s.Close()

	s2, err := vexstore.OpenAppendStore(path, testSchemaID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	records, err := s2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 1 || !bytes.Equal(records[0], record) {
		t.Fatalf("want [%q], got %v", record, records)
	}
}

func TestAppendStoreMultipleRecords(t *testing.T) {
	path := tempPath(t)
	s, err := vexstore.OpenAppendStore(path, testSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]byte{[]byte("alpha"), []byte("beta"), []byte("gamma")}
	for _, r := range want {
		if err := s.Append(r); err != nil {
			t.Fatalf("Append %q: %v", r, err)
		}
	}
	s.Close()

	s2, err := vexstore.OpenAppendStore(path, testSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("want %d records, got %d", len(want), len(got))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("record %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestAppendStoreSchemaMismatch(t *testing.T) {
	path := tempPath(t)
	s, err := vexstore.OpenAppendStore(path, testSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	other := sha256.Sum256([]byte("different-schema"))
	_, err = vexstore.OpenAppendStore(path, other)
	if err == nil {
		t.Fatal("expected schema mismatch error, got nil")
	}
}

func TestAppendStoreEmptyFile(t *testing.T) {
	path := tempPath(t)
	s, err := vexstore.OpenAppendStore(path, testSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	records, err := s.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("want 0 records, got %d", len(records))
	}
}

func TestAppendStoreFileHeader(t *testing.T) {
	path := tempPath(t)
	s, err := vexstore.OpenAppendStore(path, testSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 36 {
		t.Fatalf("file too short: %d bytes", len(raw))
	}
	// magic: VXB\0
	if raw[0] != 0x56 || raw[1] != 0x58 || raw[2] != 0x42 || raw[3] != 0x00 {
		t.Fatalf("bad magic: % x", raw[:4])
	}
	if !bytes.Equal(raw[4:36], testSchemaID[:]) {
		t.Fatal("schema hash mismatch in file")
	}
}
