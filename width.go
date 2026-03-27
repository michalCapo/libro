package main

// Width represents the configurable width of an application
type Width string

const (
	WidthMD   Width = "md"
	WidthLG   Width = "lg"
	WidthXL   Width = "xl"
	Width2XL  Width = "2xl"
	WidthFull Width = "full"
)

// AllWidths returns all available width options in order
func AllWidths() []Width {
	return []Width{WidthMD, WidthLG, WidthXL, Width2XL, WidthFull}
}

// Label returns the display label for a width
func (w Width) Label() string {
	switch w {
	case WidthMD:
		return "MD (max 640px)"
	case WidthLG:
		return "LG (max 960px)"
	case WidthXL:
		return "XL (max 1280px)"
	case Width2XL:
		return "2XL (max 1920px)"
	case WidthFull:
		return "FULL (100%)"
	default:
		return string(w)
	}
}

// PixelWidth returns the fixed pixel width for the given width tier
func (w Width) PixelWidth() string {
	switch w {
	case WidthMD:
		return "640px"
	case WidthLG:
		return "960px"
	case WidthXL:
		return "1280px"
	case Width2XL:
		return "1920px"
	case WidthFull:
		return "100%"
	default:
		return "960px"
	}
}

// PixelWidthInt returns the fixed pixel width as an integer
func (w Width) PixelWidthInt() int {
	switch w {
	case WidthMD:
		return 640
	case WidthLG:
		return 960
	case WidthXL:
		return 1280
	case Width2XL:
		return 1920
	case WidthFull:
		return 0 // dynamic, depends on viewport
	default:
		return 960
	}
}

// ContainerClasses returns Tailwind classes for the iframe container
// using the fixed pixel width the user selected.
func (w Width) ContainerClasses() string {
	if w == WidthFull {
		return "w-full shrink-0"
	}
	return "w-[" + w.PixelWidth() + "] shrink-0"
}
