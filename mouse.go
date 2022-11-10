package tea

import (
	"bytes"
	"errors"
	"strconv"
)

const x10ByteOffset = 32

// MouseMsg contains information about a mouse event and are sent to a programs
// update function when mouse activity occurs. Note that the mouse must first
// be enabled via in order the mouse events to be received.
type MouseMsg MouseEvent

// MouseEvent represents a mouse event, which could be a click, a scroll wheel
// movement, a cursor movement, or a combination.
type MouseEvent struct {
	X       int
	Y       int
	Type    MouseEventType
	Shift   bool
	Alt     bool
	Ctrl    bool
	Release bool // true if the mouse button was released (SGR only)
}

// String returns a string representation of a mouse event.
func (m MouseEvent) String() (s string) {
	if m.Ctrl {
		s += "ctrl+"
	}
	if m.Alt {
		s += "alt+"
	}
	if m.Shift {
		s += "shift+"
	}
	s += mouseEventTypes[m.Type]
	// Only SGR mouse events report button releases.
	if m.Release {
		s += " release"
	}
	return s
}

// MouseEventType indicates the type of mouse event occurring.
type MouseEventType int

// Mouse event types.
const (
	MouseUnknown MouseEventType = iota
	MouseLeft
	MouseRight
	MouseMiddle
	MouseRelease // mouse button release (X10 only)
	MouseWheelUp
	MouseWheelDown
	MouseMotion
)

var mouseEventTypes = map[MouseEventType]string{
	MouseUnknown:   "unknown",
	MouseLeft:      "left",
	MouseRight:     "right",
	MouseMiddle:    "middle",
	MouseRelease:   "release",
	MouseWheelUp:   "wheel up",
	MouseWheelDown: "wheel down",
	MouseMotion:    "motion",
}

func parseMouseEvents(buf []byte) ([]MouseEvent, error) {
	if len(buf) == 0 {
		return nil, errors.New("empty buffer")
	}

	switch {
	case bytes.Contains(buf, []byte("\x1b[<")):
		return parseSGRMouseEvents(buf)
	case bytes.Contains(buf, []byte("\x1b[M")):
		return parseX10MouseEvents(buf)
	}

	return nil, errors.New("not a mouse event")
}

// parseSGRMouseEvents parses SGR extended mouse events. SGR mouse events look
// like:
//
//	ESC [ < Cb ; Cx ; Cy (M or m)
//
// where:
//
//	Cb is the encoded button code
//	Cx is the x-coordinate of the mouse
//	Cy is the y-coordinate of the mouse
//	M is for button press, m is for button release
//
// https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#h3-Extended-coordinates
func parseSGRMouseEvents(buf []byte) ([]MouseEvent, error) {
	var r []MouseEvent

	seq := []byte("\x1b[<")
	if !bytes.Contains(buf, seq) {
		return nil, errors.New("not a SGR mouse event")
	}

	for _, v := range bytes.Split(buf, seq) {
		if len(v) == 0 {
			continue
		}

		e := bytes.Split(v, []byte(";"))
		if len(e) != 3 {
			return nil, errors.New("not a SGR mouse event")
		}

		b, _ := strconv.Atoi(string(e[0]))
		m := parseMouseButton(b, true)
		m.Release = e[2][len(e[2])-1] == 'm'

		px := e[1]
		py := e[2][:len(e[2])-1]
		x, _ := strconv.Atoi(string(px))
		y, _ := strconv.Atoi(string(py))

		// (1,1) is the upper left. We subtract 1 to normalize it to (0,0).
		m.X = x - 1
		m.Y = y - 1

		r = append(r, m)
	}

	return r, nil
}

// Parse X10-encoded mouse events; the simplest kind. The last release of X10
// was December 1986, by the way. The original X10 mouse protocol limits the Cx
// and Cy ordinates to 223 (=255 - 32).
//
// X10 mouse events look like:
//
//	ESC [M Cb Cx Cy
//
// See: http://www.xfree86.org/current/ctlseqs.html#Mouse%20Tracking
func parseX10MouseEvents(buf []byte) ([]MouseEvent, error) {
	var r []MouseEvent

	seq := []byte("\x1b[M")
	if !bytes.Contains(buf, seq) {
		return r, errors.New("not an X10 mouse event")
	}

	for _, v := range bytes.Split(buf, seq) {
		if len(v) == 0 {
			continue
		}
		if len(v) != 3 {
			return r, errors.New("not an X10 mouse event")
		}

		m := parseMouseButton(int(v[0]), false)

		// (1,1) is the upper left. We subtract 1 to normalize it to (0,0).
		m.X = int(v[1]) - x10ByteOffset - 1
		m.Y = int(v[2]) - x10ByteOffset - 1

		r = append(r, m)
	}

	return r, nil
}

func parseMouseButton(b int, isSGR bool) MouseEvent {
	var m MouseEvent
	e := b
	if !isSGR {
		e -= x10ByteOffset
	}

	const (
		bitShift  = 0b0000_0100
		bitAlt    = 0b0000_1000
		bitCtrl   = 0b0001_0000
		bitMotion = 0b0010_0000
		bitWheel  = 0b0100_0000

		bitsMask = 0b0000_0011

		bitsLeft    = 0b0000_0000
		bitsMiddle  = 0b0000_0001
		bitsRight   = 0b0000_0010
		bitsRelease = 0b0000_0011

		bitsWheelUp   = 0b0000_0000
		bitsWheelDown = 0b0000_0001
	)

	if e&bitWheel != 0 {
		// Check the low two bits.
		switch e & bitsMask {
		case bitsWheelUp:
			m.Type = MouseWheelUp
		case bitsWheelDown:
			m.Type = MouseWheelDown
		}
	} else {
		// Check the low two bits.
		// We do not separate clicking and dragging.
		switch e & bitsMask {
		case bitsLeft:
			m.Type = MouseLeft
		case bitsMiddle:
			m.Type = MouseMiddle
		case bitsRight:
			m.Type = MouseRight
		case bitsRelease:
			if e&bitMotion != 0 {
				m.Type = MouseMotion
			} else {
				m.Type = MouseRelease
			}
		}
	}

	m.Alt = e&bitAlt != 0
	m.Ctrl = e&bitCtrl != 0
	m.Shift = e&bitShift != 0

	return m
}
