package tui

import (
	"os"
	"unicode/utf8"
)

// Key constants for keys that carry no printable rune.
const (
	KeyArrowUp = 1000 + iota
	KeyArrowDown
	KeyArrowLeft
	KeyArrowRight
	KeyEnter
	KeyBackspace
)

// readKeys starts a goroutine that decodes stdin into key events in the
// same shape as BM: runes for printable keys, constants above otherwise.
func readKeys() <-chan rune {
	ch := make(chan rune, 16)
	go func() {
		defer close(ch)
		reader := newKeyReader(os.Stdin)
		for {
			key, ok := reader.next()
			if !ok {
				return
			}
			ch <- key
		}
	}()
	return ch
}

// keyReader decodes raw terminal input one key at a time.
type keyReader struct {
	f    *os.File
	buf  []byte
	size int
	pos  int
}

func newKeyReader(f *os.File) *keyReader {
	return &keyReader{f: f, buf: make([]byte, 256)}
}

// next returns the next decoded key and whether decoding may continue.
func (r *keyReader) next() (rune, bool) {
	b, ok := r.readByte()
	if !ok {
		return 0, false
	}
	switch {
	case b == 0x1b:
		return r.escapeSequence()
	case b == '\r':
		return KeyEnter, true
	case b == '\t':
		return '\t', true
	case b == 0x7f || b == 0x08:
		return KeyBackspace, true
	case b == 0x03:
		return rune(b), true
	case b < 0x20:
		return r.next()
	case b < 0x80:
		return rune(b), true
	}
	seq := []byte{b}
	for range utf8RuneLen(b) - 1 {
		c, ok := r.readByte()
		if !ok {
			return 0, false
		}
		seq = append(seq, c)
	}
	rr, _ := utf8.DecodeRune(seq)
	return rr, true
}

// escapeSequence decodes an escape sequence and returns the key it maps
// to. A lone escape byte with no follower buffered already is the escape
// key itself; unknown sequences are consumed and skipped.
func (r *keyReader) escapeSequence() (rune, bool) {
	if r.pos >= r.size {
		return 0x1b, true
	}
	b, ok := r.readByte()
	if !ok {
		return 0, false
	}
	if b != '[' {
		return 0x1b, true
	}
	for {
		c, ok := r.readByte()
		if !ok {
			return 0, false
		}
		switch c {
		case 'A':
			return KeyArrowUp, true
		case 'B':
			return KeyArrowDown, true
		case 'C':
			return KeyArrowRight, true
		case 'D':
			return KeyArrowLeft, true
		case 'Z':
			return '\t', true
		}
		if c >= 0x40 && c <= 0x7e {
			return r.next()
		}
	}
}

func (r *keyReader) readByte() (byte, bool) {
	if r.pos >= r.size {
		n, err := r.f.Read(r.buf)
		if err != nil || n == 0 {
			return 0, false
		}
		r.size = n
		r.pos = 0
	}
	b := r.buf[r.pos]
	r.pos++
	return b, true
}

// utf8RuneLen returns the byte length of the rune starting with b.
func utf8RuneLen(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b&0xe0 == 0xc0:
		return 2
	case b&0xf0 == 0xe0:
		return 3
	case b&0xf8 == 0xf0:
		return 4
	}
	return 1
}
