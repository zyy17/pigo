// Hardware cursor for the composer.
//
// Bubble Tea v2 places it from the view. The position is a cell in the
// frame, origin at the top left. A steady bar sits in the gap before the
// next cell, which is where an IME candidate window anchors.

package cli

import tea "charm.land/bubbletea/v2"

func placeCursor(v *tea.View, x, y int, show bool) {
	if !show {
		return
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	cur := tea.NewCursor(x, y)
	cur.Shape = tea.CursorBar
	cur.Blink = false
	v.Cursor = cur
}
