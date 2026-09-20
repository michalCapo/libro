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

func TestToggleMaxWidthRestoresPreviousSize(t *testing.T) {
	for _, initial := range []Width{WidthXS, WidthSM, WidthMD, WidthLG, WidthXL, Width2XL} {
		t.Run(string(initial), func(t *testing.T) {
			sm := NewStateManager()
			s := &AppState{Apps: []Application{{ID: "panel", Width: initial}, {ID: "other", Width: WidthSM}}}
			sm.states["test"] = s
			for range 2 {
				for _, want := range []Width{WidthFull, initial} {
					width, id := sm.ToggleMaxWidth("test", 1920)
					if width != want || id != "panel" || s.Apps[0].Width != want {
						t.Fatalf("toggle = (%s, %s), want (%s, panel)", width, id, want)
					}
				}
			}
			if s.Apps[1].Width != WidthSM {
				t.Fatal("toggle changed another panel's width")
			}
		})
	}
}
