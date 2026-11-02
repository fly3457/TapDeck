//go:build windows

package video

import "testing"

func TestPrimaryDisplayIsFound(t *testing.T) {
	d, err := PrimaryDisplay()
	if err != nil {
		t.Skip(err)
	}
	if d.Width <= 0 || d.Height <= 0 {
		t.Fatalf("%+v", d)
	}
	t.Logf("primary display %+v", d)
	x, y, visible := CursorOn(d)
	t.Logf("cursor %d,%d visible=%v", x, y, visible)
}
