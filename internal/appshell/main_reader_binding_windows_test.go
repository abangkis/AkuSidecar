//go:build windows && auth_session_dev

package appshell

import "testing"

func TestMainReaderBindingUsesExistingContainmentRegistry(t *testing.T) {
	c := &captureZOrder{retainReaderVisibility: true, readers: map[uintptr]captureReaderBinding{}, readerLifetimes: map[uintptr]struct{}{}}
	w := &Window{containment: c}
	b, err := w.BorrowMainReaderBinding()
	if err != nil || b.containment != c {
		t.Fatal("main reader created a competing registry")
	}
	b.containment.readerLifetimes[123] = struct{}{}
	if _, ok := c.readerLifetimes[123]; !ok {
		t.Fatal("reader lifetime not shared")
	}
	c.retainReaderVisibility = false
	if _, err = w.BorrowMainReaderBinding(); err == nil {
		t.Fatal("ordinary reader mode silently adopted")
	}
}
