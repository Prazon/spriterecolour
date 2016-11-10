package recolour

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sort"

	colorful "github.com/lucasb-eyer/go-colorful"
	// This causes the codecs to be loaded
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
)

var EPSILON float64 = 0.00000001

func floatEquals(a, b float64) bool {
	if (a-b) < EPSILON && (b-a) < EPSILON {
		return true
	}
	return false
}

type UniqueColour struct {
	RGBA    color.RGBA
	H, S, V float64
	// Store an index so that references in map know final position in list
	Index uint16
}

type UniqueColourList []*UniqueColour

func (l UniqueColourList) Len() int {
	return len(l)
}

func (l UniqueColourList) Less(i, j int) bool {
	a := l[i]
	b := l[j]
	if floatEquals(a.H, b.H) {
		if floatEquals(a.S, b.S) {
			return a.V < b.V
		}
		return a.S < b.S
	}
	return a.H < b.H
}

func (l UniqueColourList) Swap(i, j int) {
	l[i].Index, l[j].Index = l[j].Index, l[i].Index
	l[i], l[j] = l[j], l[i]
}

// Why the hell doesn't image/color have  path for this? Only the reverse
func colourTo8BitRGBA(c color.Color) color.RGBA {
	// color.Color.RGBA is 0-65535 even from 8-bit channel images because of course it is
	// Also premultiplied alpha but we'll preserve that if present
	r, g, b, a := c.RGBA()
	return color.RGBA{
		uint8((float64(r) / 65535.0) * 255.0),
		uint8((float64(g) / 65535.0) * 255.0),
		uint8((float64(b) / 65535.0) * 255.0),
		uint8((float64(a) / 65535.0) * 255.0),
	}

}

func colourTo8BitPaletteRGBA(c color.Color) color.RGBA {
	cout := colourTo8BitRGBA(c)
	// Palette entries must be solid
	cout.A = 255
	return cout
}

// Generate reads an input sprite texture and generates a reference sprite file,
// and a base lookup texture and / or parameter list
func Generate(imagePath, outImagePath, outPaletteTexture string) ([]color.RGBA, error) {

	f, err := os.OpenFile(imagePath, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	return GenerateFromImage(img, outImagePath, outPaletteTexture)
}

// GenerateFromImage reads an image and generates a reference sprite file,
// and a base lookup texture and / or parameter list
func GenerateFromImage(img image.Image, outImagePath, outPaletteTexture string) ([]color.RGBA, error) {
	bounds := img.Bounds()
	// Record of what colours are present
	colourMap := make(map[color.RGBA]*UniqueColour)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// Go colours are alpha-premultiplied and uint32's with 65535 range: weird
			// We want NON alpha premultiplied by default (internally could be premultiplied)
			p := colourTo8BitPaletteRGBA(img.At(x, y))

			if _, ok := colourMap[p]; !ok {
				cfcol := colorful.Color{float64(p.R) / 255.0, float64(p.G) / 255.0, float64(p.B) / 255.0}
				h, s, v := cfcol.Hsv()
				col := &UniqueColour{p, h, s, v, 0}
				colourMap[p] = col
			}
		}
	}

	if len(colourMap) > 65536 {
		return nil, fmt.Errorf("Sorry, sprite contains too many colours")
	}

	// Re-order the colours by HSV so easier to edit
	colourList := make(UniqueColourList, 0, len(colourMap))
	nextIndex := uint16(0)
	for _, c := range colourMap {
		c.Index = nextIndex
		colourList = append(colourList, c)
		nextIndex++
	}
	// Sort, the swap function will swap indexes
	sort.Sort(colourList)

	// Now generate the sprite output
	outSprite := image.NewNRGBA(image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			inpix := colourTo8BitPaletteRGBA(img.At(x, y))

			// Should never fail but just don't write pixel if it does
			if col, ok := colourMap[inpix]; ok {
				// Red channel = colour index U
				red := uint8(col.Index & 0x0000FFFF)
				// Blue channel = colour index V
				blue := uint8(col.Index >> 16)
				// Green channel = unused for now
				outSprite.Set(x, y, color.RGBA{red, blue, 0, inpix.A})
			}
		}
	}
	of, err := os.OpenFile(outImagePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	err = png.Encode(of, outSprite)
	of.Close()
	if err != nil {
		return nil, err
	}

	// Now write palette texture & build return
	palette := make([]color.RGBA, 0, len(colourList))
	if len(outPaletteTexture) > 0 {
		width, height := getPaletteImageDimensions(len(colourList))
		outPalette := image.NewRGBA(image.Rect(0, 0, width, height))
		x := 0
		y := 0
		for n := 0; n < len(colourList); n++ {
			outPalette.SetRGBA(x, y, colourList[n].RGBA)
			palette = append(palette, colourList[n].RGBA)
			x++
			if x == width {
				x = 0
				y++
			}
		}

		opf, err := os.OpenFile(outPaletteTexture, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}
		err = png.Encode(opf, outPalette)
		opf.Close()
		if err != nil {
			return nil, err
		}

	}

	return palette, nil
}

func nextPowerOfTwo(v int) int {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}

func getPaletteImageDimensions(numColours int) (width, height int) {
	width = 256
	height = 1
	if numColours > 256 {
		height = nextPowerOfTwo(int(math.Ceil(float64(numColours) / 256.0)))
	} else if numColours <= 128 {
		width = nextPowerOfTwo(numColours)
	}
	return
}
