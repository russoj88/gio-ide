package terminal

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
)

func TestEmbeddedMonoIsCrisp(t *testing.T) {
	raw, err := opentype.Parse(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(raw, &opentype.FaceOptions{
		Size:    13,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		t.Fatal(err)
	}
	f := &gridFont{
		px:       13,
		baseline: face.Metrics().Ascent.Ceil(),
		linePx:   face.Metrics().Ascent.Ceil() + face.Metrics().Descent.Ceil(),
		faces:    [4]font.Face{face, face, face, face},
		masks:    map[maskKey]glyphMask{},
	}
	adv, _ := face.GlyphAdvance('M')
	f.cellW = adv.Round()
	m, ok := f.glyphLocked(0, 'M')
	if !ok {
		t.Fatal("no M")
	}
	solid := 0
	for _, a := range m.alpha.Pix {
		if a == 255 {
			solid++
		}
	}
	if solid < 8 {
		t.Fatalf("Go Mono M has %d solid pixels", solid)
	}
	if m.bounds.Min.Y < 0 || m.bounds.Max.Y > f.linePx {
		t.Fatalf("M bounds %v escape line %d", m.bounds, f.linePx)
	}
}

func TestGlyphsSitOnThePixelGrid(t *testing.T) {
	f, err := fontFor(13)
	if err != nil {
		t.Fatal(err)
	}
	if f.cellW < 5 || f.linePx <= f.baseline || f.baseline < 1 {
		t.Fatalf("metrics cell=%d line=%d baseline=%d", f.cellW, f.linePx, f.baseline)
	}
	cells := []Cell{{R: 'M', W: 1}, {R: 'W', W: 1}}
	img := image.NewRGBA(image.Rect(0, 0, f.cellW*2, f.linePx))
	st := Style{
		Foreground: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		Background: color.NRGBA{A: 255},
	}
	fillRow(13, img, cells, 2, st, -1, st.Background)

	maxL, minR := -1, img.Bounds().Dx()+1
	minY, maxY := img.Bounds().Dy(), -1
	solidL, solidR := 0, 0
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if img.RGBAAt(x, y).A < 250 {
				continue
			}
			if x < f.cellW {
				solidL++
				if x > maxL {
					maxL = x
				}
			} else {
				solidR++
				if x < minR {
					minR = x
				}
			}
			if x < f.cellW {
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if solidL < 8 || solidR < 8 {
		t.Fatalf("solid pixels left=%d right=%d, want stems on both cells", solidL, solidR)
	}
	if maxL >= f.cellW || minR < f.cellW {
		t.Fatalf("solid ink crosses the cell edge: left max %d right min %d cell %d", maxL, minR, f.cellW)
	}
	if minY >= f.baseline || maxY <= 1 || maxY >= f.linePx {
		t.Fatalf("M ink y %d..%d, baseline %d line %d", minY, maxY, f.baseline, f.linePx)
	}
}
