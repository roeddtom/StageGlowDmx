package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func testDirectory(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".config-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestStoreRoundTripAndExplicitRecovery(t *testing.T) {
	dir := testDirectory(t)
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.Get(), Defaults()) {
		t.Fatal("unsafe or malformed defaults")
	}
	initial, _ := os.ReadFile(store.path)
	c := rig()
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(store.path + ".bak")
	if err != nil || !bytes.Equal(backup, initial) {
		t.Fatal("backup does not retain previous valid file", err)
	}
	reopened, err := Open(dir)
	if err != nil || !reflect.DeepEqual(reopened.Get(), c) {
		t.Fatal("round-trip failed", err)
	}
	// Mutating caller-owned state or a snapshot cannot change stored state.
	c.Profiles[0].Name = "caller mutation"
	snapshot := store.Get()
	snapshot.Profiles[0].Channels[0].Kind = "fixed"
	snapshot.Loadouts[0].Fixtures[0].Name = "snapshot mutation"
	if !reflect.DeepEqual(store.Get(), rig()) {
		t.Fatal("store exposes shared mutable state")
	}
	if err := os.WriteFile(store.path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "restore") {
		t.Fatalf("corrupt startup should give recovery guidance: %v", err)
	}
	if err := store.Save(Defaults()); err == nil {
		t.Fatal("save overwrote externally corrupted primary")
	}
	unchanged, _ := os.ReadFile(store.path + ".bak")
	if !bytes.Equal(backup, unchanged) || !reflect.DeepEqual(store.Get(), rig()) {
		t.Fatal("failure changed backup or live state")
	}
	if err := os.WriteFile(store.path, backup, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(dir)
	if err != nil || !reflect.DeepEqual(recovered.Get(), Defaults()) {
		t.Fatal("explicit backup recovery failed", err)
	}
}

func TestStoreFailureLeavesStateUnchanged(t *testing.T) {
	dir := testDirectory(t)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path)
	bad := rig()
	bad.BPM = 0
	if err := s.Save(bad); err == nil {
		t.Fatal("invalid save accepted")
	}
	// A directory at the backup path deterministically prevents backup replacement.
	if err := os.Mkdir(s.path+".bak", 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(rig()); err == nil {
		t.Fatal("filesystem failure ignored")
	}
	after, _ := os.ReadFile(s.path)
	if !bytes.Equal(before, after) || !reflect.DeepEqual(s.Get(), Defaults()) {
		t.Fatal("failed save changed primary or memory")
	}
}

func TestMissingPrimaryWithBackupDoesNotCreateDefaults(t *testing.T) {
	dir := testDirectory(t)
	path := filepath.Join(dir, "config.json")
	data, _ := encode(rig())
	if err := os.WriteFile(path+".bak", data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("missing primary with backup silently defaulted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("startup wrote a new primary")
	}
}

func TestInvalidPersistedSchemaRefusesStartup(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			dir := testDirectory(t)
			s, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Save(rig()); err != nil {
				t.Fatal(err)
			}
			backup, err := os.ReadFile(s.path + ".bak")
			if err != nil {
				t.Fatal(err)
			}
			c := rig()
			c.SchemaVersion = version
			invalid, err := encode(c)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(s.path, invalid, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "unsupported schemaVersion") {
				t.Fatalf("invalid persisted schema should refuse startup: %v", err)
			}
			primaryAfter, _ := os.ReadFile(s.path)
			backupAfter, _ := os.ReadFile(s.path + ".bak")
			if !bytes.Equal(invalid, primaryAfter) || !bytes.Equal(backup, backupAfter) {
				t.Fatal("startup changed invalid primary or last-known-good backup")
			}
		})
	}
}

func TestConcurrentStoreAccess(t *testing.T) {
	s, err := Open(testDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			c := s.Get()
			c.BPM = float64(100 + i)
			if err := s.Save(c); err != nil {
				t.Error(err)
			}
			if err := s.Get().Validate(); err != nil {
				t.Error(err)
			}
		}(i)
	}
	workers.Wait()
	if _, err := Open(filepath.Dir(s.path)); err != nil {
		t.Fatal(err)
	}
}
