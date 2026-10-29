package app

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestStreamPutsAnOpenWindowBackInItsMode(t *testing.T) {
	w := &application.WebviewWindow{}
	s := &Shell{main: w, uiMode: "desktop"}
	if !s.saveForStream() || s.uiMode != "bigpicture" {
		t.Fatalf("saveForStream: open window in %q", s.uiMode)
	}
	b, got := s.restoreAfterStream()
	if b == nil || !b.open || b.mode != "desktop" || got != w || s.uiMode != "desktop" {
		t.Errorf("restoreAfterStream = %+v, %v; mode %q", b, got, s.uiMode)
	}
}

func TestStreamFromTheTrayClosesTheWindowAgain(t *testing.T) {
	s := &Shell{uiMode: "desktop"}
	if s.saveForStream() {
		t.Fatal("saveForStream: no window was open")
	}
	s.main = &application.WebviewWindow{} // EnterBigPicture opened it
	b, w := s.restoreAfterStream()
	if b == nil || b.open || w == nil || s.uiMode != "desktop" {
		t.Errorf("restoreAfterStream = %+v, %v; mode %q", b, w, s.uiMode)
	}
}

func TestSecondBigPictureKeepsTheFirstState(t *testing.T) {
	s := &Shell{uiMode: "desktop"}
	s.saveForStream()
	s.main = &application.WebviewWindow{}
	s.saveForStream()
	if b, _ := s.restoreAfterStream(); b == nil || b.open || b.mode != "desktop" {
		t.Errorf("restoreAfterStream = %+v", b)
	}
}

func TestLeaveWithoutBigPictureDoesNothing(t *testing.T) {
	s := &Shell{main: &application.WebviewWindow{}, uiMode: "bigpicture"}
	if b, w := s.restoreAfterStream(); b != nil || w != nil || s.uiMode != "bigpicture" {
		t.Errorf("restoreAfterStream = %+v, %v; mode %q", b, w, s.uiMode)
	}
	s.saveForStream()
	s.restoreAfterStream()
	if b, _ := s.restoreAfterStream(); b != nil {
		t.Errorf("second restoreAfterStream = %+v", b)
	}
}

func TestStreamEndingDuringAGameKeepsTheWindowAsRestored(t *testing.T) {
	// From the tray: the window closed for the game stays closed after it.
	s := &Shell{uiMode: "desktop"}
	s.saveForStream()
	s.gameMode = true // closed for a game started in big picture
	if _, w := s.restoreAfterStream(); w != nil || s.gameMode {
		t.Fatalf("window %v, gameMode %v", w, s.gameMode)
	}
	s.gameEnded("bigpicture")
	if s.main != nil || s.uiMode != "desktop" || s.restored {
		t.Errorf("after the game: main %v, mode %q, restored %v", s.main, s.uiMode, s.restored)
	}

	// From desktop mode: it comes back in desktop mode, not big picture.
	s = &Shell{main: &application.WebviewWindow{}, uiMode: "desktop"}
	s.saveForStream()
	s.main, s.gameMode = nil, true
	s.restoreAfterStream()
	if !s.gameMode || !s.restored || s.uiMode != "desktop" {
		t.Errorf("gameMode %v, restored %v, mode %q", s.gameMode, s.restored, s.uiMode)
	}
}
