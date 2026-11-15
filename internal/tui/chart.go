package tui

import (
	"fmt"
	"math"
	"strings"
)

// fmtBytes formats a byte count the way amdtop shows traffic values.
func fmtBytes(b uint64) string {
	const (
		TB = 1 << 40
		GB = 1 << 30
		MB = 1 << 20
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.2fTB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.0fMB", float64(b)/float64(MB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// fmtRate formats a throughput value in bytes per second and returns the
// number and the unit with kilobytes as the smallest unit.
func fmtRate(b float64) (string, string) {
	const (
		TB = 1 << 40
		GB = 1 << 30
		MB = 1 << 20
		KB = 1 << 10
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.2f", b/TB), "TB"
	case b >= GB:
		return fmt.Sprintf("%.1f", b/GB), "GB"
	case b >= MB:
		return fmt.Sprintf("%.0f", b/MB), "MB"
	case b >= 10*KB:
		return fmt.Sprintf("%.0f", b/KB), "KB"
	default:
		return fmt.Sprintf("%.1f", b/KB), "KB"
	}
}

// miniChart renders one filled history chart in half height block
// characters, the same drawing amdtop uses.
func miniChart(data []float64, width, height int, maxVal float64) string {
	if len(data) == 0 || width <= 0 || height <= 0 || maxVal <= 0 {
		return ""
	}
	n := len(data)
	offset := 0
	if n > width {
		data = data[n-width:]
		n = width
	} else {
		offset = width - n
	}

	vRes := height * 2
	pixels := make([][]bool, vRes)
	for y := range pixels {
		pixels[y] = make([]bool, width)
	}

	for x := range n {
		v := max(0, math.Min(maxVal, data[x]))
		if v <= 0 {
			continue
		}
		topY := vRes - 1 - int(v/maxVal*float64(vRes-1))
		topY = max(topY, 0)
		for dy := range vRes - topY {
			pixels[topY+dy][offset+x] = true
		}
	}

	var sb strings.Builder
	for row := range height {
		topRow := row * 2
		botRow := row*2 + 1
		for x := range width {
			top := pixels[topRow][x]
			bot := botRow < vRes && pixels[botRow][x]
			switch {
			case top && bot:
				sb.WriteRune('█')
			case top && !bot:
				sb.WriteRune('▀')
			case !top && bot:
				sb.WriteRune('▄')
			default:
				sb.WriteRune(' ')
			}
		}
		if row < height-1 {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

// gradientBar renders a gauge bar filled to ratio with a green to yellow
// to red ramp over the filled part.
func gradientBar(ratio float64, width int) string {
	if width <= 0 {
		return ""
	}
	ratio = max(0, min(1, ratio))
	filled := int(ratio*float64(width) + 0.5)
	var sb strings.Builder
	for i := range width {
		if i < filled {
			sb.WriteString("\x1b[38;2;")
			sb.WriteString(rampRGB(float64(i) / float64(max(width-1, 1))))
			sb.WriteString("m█")
		} else {
			sb.WriteString("\x1b[90m░")
		}
	}
	sb.WriteString("\x1b[0m")
	return sb.String()
}

// rampRGB interpolates one gauge color across the green yellow red ramp.
func rampRGB(t float64) string {
	green := [3]int{166, 218, 149}
	yellow := [3]int{238, 212, 159}
	red := [3]int{237, 135, 150}
	from, to := green, yellow
	if t > 0.5 {
		from, to = yellow, red
		t = (t - 0.5) * 2
	} else {
		t *= 2
	}
	return fmt.Sprintf("%d;%d;%d",
		from[0]+int(float64(to[0]-from[0])*t),
		from[1]+int(float64(to[1]-from[1])*t),
		from[2]+int(float64(to[2]-from[2])*t))
}

// chartMax returns the scale of a chart: the peak sample rounded up to a
// readable value and never below one kilobyte per second.
func chartMax(data []float64) float64 {
	maxVal := 1024.0
	for _, v := range data {
		if v > maxVal {
			maxVal = v
		}
	}
	pow := math.Pow(10, math.Ceil(math.Log10(maxVal))-1)
	return math.Ceil(maxVal/pow) * pow
}
