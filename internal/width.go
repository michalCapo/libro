package libro

import (
	"strconv"
	"strings"
)

// Width represents the configurable width of an application
type Width string

const (
	WidthXS   Width = "xs"
	WidthSM   Width = "sm"
	WidthMD   Width = "md"
	WidthLG   Width = "lg"
	WidthXL   Width = "xl"
	Width2XL  Width = "2xl"
	Width3XL  Width = "3xl"
	WidthFull Width = "full"
)

// AllWidths returns all available width options in order
func AllWidths() []Width {
	return []Width{WidthXS, WidthSM, WidthMD, WidthLG, WidthXL, Width2XL, Width3XL, WidthFull}
}

// Step returns the neighboring width tier clamped to the valid range.
func (w Width) Step(delta int) Width {
	widths := AllWidths()
	idx := 0
	if pixels := w.customPixels(); pixels > 0 {
		for i, candidate := range widths {
			if candidate == WidthFull || candidate.PixelWidthInt() >= pixels {
				idx = i
				if delta > 0 && candidate.PixelWidthInt() != pixels {
					idx--
				}
				break
			}
		}
	}
	for i, candidate := range widths {
		if candidate == w {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(widths) {
		idx = len(widths) - 1
	}
	return widths[idx]
}

// ShortLabel returns the compact display label for a width.
func (w Width) ShortLabel() string {
	if w == WidthFull {
		return "MAX"
	}
	return strings.ToUpper(string(w))
}

// Label returns the display label for a width
func (w Width) Label() string {
	switch w {
	case WidthXS:
		return "XS (320px)"
	case WidthSM:
		return "SM (480px)"
	case WidthMD:
		return "MD (640px)"
	case WidthLG:
		return "LG (960px)"
	case WidthXL:
		return "XL (1280px)"
	case Width2XL:
		return "2XL (1920px)"
	case Width3XL:
		return "3XL (2560px)"
	case WidthFull:
		return "MAX (100%)"
	default:
		return string(w)
	}
}

// customPixels accepts bounded pixel widths produced by dragging a panel edge.
func (w Width) customPixels() int {
	if !strings.HasSuffix(string(w), "px") {
		return 0
	}
	pixels, err := strconv.Atoi(strings.TrimSuffix(string(w), "px"))
	if err != nil || pixels < 320 || pixels > 2560 {
		return 0
	}
	return pixels
}

// PixelWidth returns the fixed pixel width for the given width tier
func (w Width) PixelWidth() string {
	if pixels := w.customPixels(); pixels > 0 {
		return strconv.Itoa(pixels) + "px"
	}
	switch w {
	case WidthXS:
		return "320px"
	case WidthSM:
		return "480px"
	case WidthMD:
		return "640px"
	case WidthLG:
		return "960px"
	case WidthXL:
		return "1280px"
	case Width2XL:
		return "1920px"
	case Width3XL:
		return "2560px"
	case WidthFull:
		return "100%"
	default:
		return "960px"
	}
}

// PixelWidthInt returns the fixed pixel width as an integer
func (w Width) PixelWidthInt() int {
	if pixels := w.customPixels(); pixels > 0 {
		return pixels
	}
	switch w {
	case WidthXS:
		return 320
	case WidthSM:
		return 480
	case WidthMD:
		return 640
	case WidthLG:
		return 960
	case WidthXL:
		return 1280
	case Width2XL:
		return 1920
	case Width3XL:
		return 2560
	case WidthFull:
		return 0 // dynamic, depends on viewport
	default:
		return 960
	}
}

// ClampFixedPixel returns the largest fixed width at or below maxPixels.
// WidthFull is treated as dynamic viewport width and is not clamped.
func (w Width) ClampFixedPixel(maxPixels int) Width {
	if maxPixels <= 0 {
		return w
	}
	if px := w.PixelWidthInt(); px == 0 || px <= maxPixels {
		return w
	}
	clamped := WidthXS
	for _, candidate := range AllWidths() {
		px := candidate.PixelWidthInt()
		if px > 0 && px <= maxPixels {
			clamped = candidate
		}
	}
	return clamped
}

// ContainerClasses returns Tailwind classes for the iframe container
// using the fixed pixel width the user selected.
func (w Width) ContainerClasses() string {
	if w.customPixels() > 0 {
		return "shrink-0" // Custom widths are applied through the inline style.
	}
	if w == WidthFull {
		return "w-full shrink-0"
	}
	return "w-[" + w.PixelWidth() + "] shrink-0"
}
