package libro

import "testing"

func TestFixedPanelWidths(t *testing.T) {
	for _, tc := range []struct {
		width  Width
		pixels int
	}{
		{WidthXS, 320}, {WidthSM, 480}, {WidthMD, 640},
		{WidthLG, 960}, {WidthXL, 1280}, {Width2XL, 1920},
	} {
		if got := tc.width.PixelWidthInt(); got != tc.pixels {
			t.Errorf("%s width = %d, want %d", tc.width, got, tc.pixels)
		}
	}
	if WidthSM.Step(-1) != WidthXS || WidthXS.Step(-1) != WidthXS || WidthXS.Step(1) != WidthSM {
		t.Fatal("width shortcuts must include XS and stop at the smallest size")
	}
	if Width3XL.Step(1) != WidthFull || WidthFull.Step(1) != WidthFull || WidthFull.Step(-1) != Width3XL {
		t.Fatal("width shortcuts must stop at the largest size")
	}
	if WidthLG.ClampFixedPixel(320) != WidthXS {
		t.Fatal("width clamp must support XS")
	}
}
