// Package media processes and stores the images of businesses and professionals (logo, cover
// and portfolio, 2026-11-27). Every upload is decoded and encoded again on the server: only
// JPEG, PNG and WebP are accepted (by content, not by name), metadata such as the GPS position
// of a phone photo is dropped, a file disguised as an image is refused, and the image is scaled
// down to a maximum side.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder
)

// Limits of an upload.
const (
	MaxUploadBytes = 5 << 20    // 5 MiB per image
	maxPixels      = 24_000_000 // decompression bombs and memory: 24 megapixels at most
	maxSide        = 12_000
	jpegQuality    = 85
)

// busy bounds the images decoded at the same time (a 24 MP photo needs tens of MB; QA has 1 GB
// shared with PostgreSQL).
var busy = make(chan struct{}, 2)

// Errors of Process.
var (
	ErrUnsupported = errors.New("media: only JPEG, PNG and WebP images are accepted")
	ErrInvalid     = errors.New("media: the image cannot be read")
	ErrTooBig      = errors.New("media: the image has too many pixels")
)

// Image is a processed image ready to be stored.
type Image struct {
	Data          []byte
	ContentType   string // image/jpeg or image/png
	Ext           string // jpg or png
	Width, Height int
}

// Process validates and encodes data again, scaled down so that no side exceeds side. Images
// with transparency stay PNG; the rest become JPEG.
func Process(data []byte, side int) (Image, error) {
	ct := http.DetectContentType(data)
	if ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" {
		return Image{}, ErrUnsupported
	}
	busy <- struct{}{}
	defer func() { <-busy }()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Image{}, ErrInvalid
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width*cfg.Height > maxPixels {
		return Image{}, ErrTooBig
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Image{}, ErrInvalid
	}
	dst := scale(src, side)
	if ct == "image/jpeg" { // rotate after scaling down: cheaper, same result
		dst = orient(dst, jpegOrientation(data))
	}
	var buf bytes.Buffer
	out := Image{Width: dst.Bounds().Dx(), Height: dst.Bounds().Dy()}
	if opaque(dst) {
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality})
		out.ContentType, out.Ext = "image/jpeg", "jpg"
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, dst)
		out.ContentType, out.Ext = "image/png", "png"
	}
	if err != nil {
		return Image{}, err
	}
	out.Data = buf.Bytes()
	return out, nil
}

// scale returns src in RGBA, scaled down (never up) so that its longest side is at most side.
func scale(src image.Image, side int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if longest := max(w, h); longest > side {
		w, h = max(1, w*side/longest), max(1, h*side/longest)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func opaque(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return false
		}
	}
	return true
}

// jpegOrientation reads the EXIF orientation (1–8) of a JPEG; 1 if absent or unreadable. The
// metadata itself is never kept: only the rotation it asks for is applied.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xff {
			return 1
		}
		marker := data[i+1]
		if marker == 0xda || marker == 0xd9 { // start of scan / end of image
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xe1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return exifOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func exifOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	ifd := int(bo.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 1
	}
	n := int(bo.Uint16(tiff[ifd:]))
	for k := range n {
		e := ifd + 2 + k*12
		if e+12 > len(tiff) {
			return 1
		}
		if bo.Uint16(tiff[e:]) == 0x0112 { // Orientation, SHORT
			if v := int(bo.Uint16(tiff[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient applies an EXIF orientation so the image shows as the camera meant it.
func orient(src *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
