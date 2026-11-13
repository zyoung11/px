package tui

import (
	"fmt"
	"math"
	"strings"
)

// chartColor is the chart accent, amdtop's default chart color.
const chartColor = "\x1b[38;2;230;81;0m"

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
