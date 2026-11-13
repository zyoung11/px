package progress

import (
	"fmt"
	"io"
	"time"
)

// step is the minimum interval between two progress reports.
const step = 500 * time.Millisecond

// Copy copies src into dst and reports the transferred share through
// report. A total below zero disables the share reports.
func Copy(dst io.Writer, src io.Reader, total int64, label string, report func(string)) (int64, error) {
	buf := make([]byte, 32*1024)
	written := int64(0)
	last := time.Time{}
	for {
		n, err := src.Read(buf)
		if n > 0 {
			w, werr := dst.Write(buf[:n])
			written += int64(w)
			if werr != nil {
				return written, werr
			}
			if report != nil && total > 0 && time.Since(last) >= step {
				last = time.Now()
				report(fmt.Sprintf("%s %d%%", label, written*100/total))
			}
		}
		if err == io.EOF {
			return written, nil
		}
		if err != nil {
			return written, err
		}
	}
}
