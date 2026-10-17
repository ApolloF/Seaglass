package vpad

import "testing"

// Looking only reads, so it's safe on any PC, with or without the drivers.
func TestGetStatusOnlyLooks(t *testing.T) {
	s := GetStatus()
	t.Logf("%+v", s)
	if s.CanHide && !s.HidHide {
		t.Errorf("can hide without HidHide: %+v", s)
	}
}
