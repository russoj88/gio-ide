package terminal

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// gridFont is one pixel size of the terminal face. Glyphs are rasterized
// once and blitted on the cell grid. The UI goroutine is the only caller;
// sizeMu also covers tests.
type gridFont struct {
	px, cellW, linePx, baseline int
	faces                       [4]font.Face
	masks                       map[maskKey]glyphMask
}

type maskKey struct {
	face uint8
	r    rune
}

// glyphMask is the coverage of one rune. bounds is relative to the cell origin.
type glyphMask struct {
	alpha  *image.Alpha
	bounds image.Rectangle
}

var (
	sizeMu   sync.Mutex
	sizes    = map[int]*gridFont{}
	fontOnce sync.Once
	rawFonts [4]*opentype.Font
	fontErr  error
	lutOnce  sync.Once
	coverLUT [256]byte
)

// dejavu and liberation are the monospace families a Linux terminal uses.
// Go Mono is the fallback when those files are not installed.
var monoFiles = [][4]string{
	{
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Oblique.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-BoldOblique.ttf",
	},
	{
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Bold.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Italic.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-BoldItalic.ttf",
	},
}

func ensureFonts() error {
	fontOnce.Do(func() {
		for _, paths := range monoFiles {
			faces, ok := readFamily(paths)
			if ok {
				rawFonts = faces
				return
			}
		}
		blobs := [][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF}
		for i, b := range blobs {
			f, err := opentype.Parse(b)
			if err != nil {
				fontErr = fmt.Errorf("terminal font: %w", err)
				return
			}
			rawFonts[i] = f
		}
	})
	return fontErr
}

func readFamily(paths [4]string) ([4]*opentype.Font, bool) {
	var out [4]*opentype.Font
	for i, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return out, false
		}
		f, err := opentype.Parse(b)
		if err != nil {
			return out, false
		}
		out[i] = f
	}
	return out, true
}

// coverageLUT snaps stem cores to solid pixels and drops the faint halo
// that makes an unhinted outline look soft at terminal sizes. Edge pixels
// stay gray so curves keep one pixel of antialiasing.
func coverageLUT() *[256]byte {
	lutOnce.Do(func() {
		for i := range 256 {
			a := byte(i)
			if a < 16 {
				continue
			}
			y := math.Pow(float64(a)/255, 0.5)
			if y >= 0.82 {
				coverLUT[i] = 255
				continue
			}
			coverLUT[i] = byte(y*255 + 0.5)
		}
	})
	return &coverLUT
}

func fontFor(px int) (*gridFont, error) {
	sizeMu.Lock()
	defer sizeMu.Unlock()
	return fontForLocked(px)
}

func fontForLocked(px int) (*gridFont, error) {
	if px < 1 {
		px = 1
	}
	if f, ok := sizes[px]; ok {
		return f, nil
	}
	if err := ensureFonts(); err != nil {
		return nil, err
	}
	var faces [4]font.Face
	for i, raw := range rawFonts {
		face, err := opentype.NewFace(raw, &opentype.FaceOptions{
			Size:    float64(px),
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			return nil, fmt.Errorf("terminal font: %w", err)
		}
		faces[i] = face
	}
	adv, ok := faces[0].GlyphAdvance('M')
	cellW := adv.Round()
	if !ok || cellW < 1 {
		cellW = max(px/2, 1)
	}
	met := faces[0].Metrics()
	ascent := met.Ascent.Ceil()
	descent := met.Descent.Ceil()
	if ascent < 1 {
		ascent = px
	}
	if descent < 1 {
		descent = 1
	}
	f := &gridFont{
		px:       px,
		cellW:    cellW,
		linePx:   ascent + descent,
		baseline: ascent,
		faces:    faces,
		masks:    map[maskKey]glyphMask{},
	}
	sizes[px] = f
	return f, nil
}

func metricsOf(px int) (cellW, linePx int, ok bool) {
	f, err := fontFor(px)
	if err != nil {
		return 0, 0, false
	}
	return f.cellW, f.linePx, true
}

func faceIndex(a Attr) int {
	n := 0
	if a.Bold {
		n = 1
	}
	if a.Italic {
		n += 2
	}
	return n
}

func (f *gridFont) glyphLocked(face int, r rune) (glyphMask, bool) {
	if face < 0 || face > 3 {
		face = 0
	}
	key := maskKey{uint8(face), r}
	if m, ok := f.masks[key]; ok {
		return m, m.alpha != nil
	}
	var m glyphMask
	dr, src, mp, _, ok := f.faces[face].Glyph(fixed.P(0, f.baseline), r)
	if ok && src != nil && !dr.Empty() {
		m.bounds = dr
		m.alpha = copyCoverage(src, dr, mp)
	}
	f.masks[key] = m
	return m, m.alpha != nil
}

func copyCoverage(src image.Image, dr image.Rectangle, mp image.Point) *image.Alpha {
	w, h := dr.Dx(), dr.Dy()
	if w < 1 || h < 1 || w > 512 || h > 512 {
		return nil
	}
	dst := image.NewAlpha(image.Rect(0, 0, w, h))
	lut := coverageLUT()
	switch src := src.(type) {
	case *image.Alpha:
		for y := range h {
			for x := range w {
				p := mp.Add(image.Pt(x, y))
				dst.Pix[dst.PixOffset(x, y)] = lut[src.AlphaAt(p.X, p.Y).A]
			}
		}
	default:
		for y := range h {
			for x := range w {
				p := mp.Add(image.Pt(x, y))
				_, _, _, a := src.At(p.X, p.Y).RGBA()
				dst.Pix[dst.PixOffset(x, y)] = lut[byte(a>>8)]
			}
		}
	}
	return dst
}

// fillRow paints cols glyphs into dst. dst's origin is the top-left of the row.
// cursorX recolors that cell so a block cursor stays readable. Pass -1 for none.
func fillRow(px int, dst *image.RGBA, cells []Cell, cols int, style Style, cursorX int, contrast color.NRGBA) {
	sizeMu.Lock()
	defer sizeMu.Unlock()
	f, err := fontForLocked(px)
	if err != nil {
		return
	}
	n := min(cols, len(cells))
	for x := 0; x < n; {
		c := cells[x]
		if c.W < 0 {
			x++
			continue
		}
		w := 1
		if c.W == 2 {
			w = 2
		}
		if c.R != 0 {
			m, ok := f.glyphLocked(faceIndex(c.A), c.R)
			if ok {
				fg, _ := styleColors(style, c.A)
				if x == cursorX {
					fg = contrast
				}
				blitGlyph(dst, m, image.Pt(x*f.cellW, 0), fg)
			}
		}
		x += w
	}
}

func blitGlyph(dst *image.RGBA, m glyphMask, origin image.Point, col color.NRGBA) {
	if m.alpha == nil || col.A == 0 {
		return
	}
	dr := m.bounds.Add(origin).Intersect(dst.Bounds())
	if dr.Empty() {
		return
	}
	cr := uint32(col.R) * uint32(col.A) / 255
	cg := uint32(col.G) * uint32(col.A) / 255
	cb := uint32(col.B) * uint32(col.A) / 255
	ca := uint32(col.A)
	for y := dr.Min.Y; y < dr.Max.Y; y++ {
		for x := dr.Min.X; x < dr.Max.X; x++ {
			sx := x - (origin.X + m.bounds.Min.X)
			sy := y - (origin.Y + m.bounds.Min.Y)
			a := uint32(m.alpha.AlphaAt(sx, sy).A)
			if a == 0 {
				continue
			}
			sa := a * ca / 255
			sr := a * cr / 255
			sg := a * cg / 255
			sb := a * cb / 255
			i := dst.PixOffset(x, y)
			inv := 255 - sa
			dst.Pix[i+0] = uint8(sr + uint32(dst.Pix[i+0])*inv/255)
			dst.Pix[i+1] = uint8(sg + uint32(dst.Pix[i+1])*inv/255)
			dst.Pix[i+2] = uint8(sb + uint32(dst.Pix[i+2])*inv/255)
			dst.Pix[i+3] = uint8(sa + uint32(dst.Pix[i+3])*inv/255)
		}
	}
}
