package vpad

import (
	"os"
	"path/filepath"
	"testing"
)

// Looking only reads, so it's safe on any PC, with or without the drivers.
func TestGetStatusOnlyLooks(t *testing.T) {
	s := GetStatus(filepath.Join(t.TempDir(), "hidden-controllers.json"))
	t.Logf("%+v", s)
	if s.CanHide && !s.HidHide {
		t.Errorf("can hide without HidHide: %+v", s)
	}
}

// The ledger remembers exactly what Seaglass hid, survives a restart and
// goes once everything in it is shown again.
func TestLedgerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hidden-controllers.json")
	if Hidden(path) {
		t.Fatal("hidden without a ledger")
	}
	ids := []string{`HID\VID_054C&PID_0CE6&MI_03\7&1A2B&0&0000`}
	if err := writeLedger(path, ids); err != nil {
		t.Fatal(err)
	}
	if !Hidden(path) || !containsFold(readLedger(path), `hid\vid_054c&pid_0ce6&mi_03\7&1a2b&0&0000`) {
		t.Fatalf("ledger = %q", readLedger(path))
	}
	if err := removeLedger(path); err != nil {
		t.Fatal(err)
	}
	if err := removeLedger(path); err != nil {
		t.Errorf("removing a missing ledger: %v", err)
	}
	if Hidden(path) {
		t.Error("still hidden after the ledger went")
	}
}

// A damaged ledger lists nothing rather than failing the launch.
func TestDamagedLedgerListsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hidden-controllers.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Hidden(path) {
		t.Error("a damaged ledger counted as hidden controllers")
	}
}
