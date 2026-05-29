package libro

// Width represents the configurable width of an application
type Width string

const (
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
	return []Width{WidthSM, WidthMD, WidthLG, WidthXL, Width2XL, Width3XL, WidthFull}
}

// Step returns the neighboring width tier clamped to the valid range.
func (w Width) Step(delta int) Width {
	widths := AllWidths()
	idx := 0
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

// Label returns the display label for a width
func (w Width) Label() string {
	switch w {
	case WidthSM:
		return "SM (max 480px)"
	case WidthMD:
		return "MD (max 640px)"
	case WidthLG:
		return "LG (max 960px)"
	case WidthXL:
		return "XL (max 1280px)"
	case Width2XL:
		return "2XL (max 1920px)"
	case Width3XL:
		return "3XL (max 2560px)"
	case WidthFull:
		return "FULL (100%)"
	default:
		return string(w)
	}
}

// PixelWidth returns the fixed pixel width for the given width tier
func (w Width) PixelWidth() string {
	switch w {
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
	switch w {
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
	clamped := WidthSM
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
	if w == WidthFull {
		return "w-full shrink-0"
	}
	return "w-[" + w.PixelWidth() + "] shrink-0"
}
