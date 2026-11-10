package mcpserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
)

const (
	maxMediaProbeBytes = 1 << 20
	maxMediaProbeBoxes = 4096
	// Only root -> moov -> trak -> mdia is traversed. Other containers are
	// skipped, so attacker-controlled nesting cannot drive recursion.
	maxMediaProbeDepth = 4
)

var (
	errMediaProbeInvalid = errors.New("local_media_metadata_invalid")
	errMediaProbeLimit   = errors.New("local_media_probe_limit_exceeded")
	errMediaProbeRead    = errors.New("local_media_probe_unavailable: cannot read local media")
)

// probeLocalMedia reads headers from an already validated, open local file.
// It neither opens paths nor changes the caller's file position. All fields
// remain null on malformed metadata or exhausted budgets; unsupported formats
// are ordinary warning results, including legacy fake mp4 files containing
// "video". Only cancellation and local I/O failures return errors.
//
// duration_seconds is float64; width/height are int; resolution is "WxH".
// metadata_sources is map[string]string and contains only populated fields:
// mp4_mvhd, mp4_tkhd, or image_header. No media payload is decoded.
func probeLocalMedia(ctx context.Context, file *os.File, size int64) (map[string]any, []string, error) {
	data := map[string]any{
		"duration_seconds": nil,
		"width":            nil,
		"height":           nil,
		"resolution":       nil,
		"metadata_sources": map[string]string{},
	}
	if err := ctx.Err(); err != nil {
		return data, nil, err
	}
	if file == nil || size < 0 {
		return mediaProbeFailure(ctx, data, errMediaProbeRead)
	}
	p := mediaProbe{ctx: ctx, file: file, size: size, remaining: maxMediaProbeBytes}
	var magic [12]byte
	head := magic[:min(size, int64(len(magic)))]
	if err := p.readAt(head, 0); err != nil {
		return mediaProbeFailure(ctx, data, err)
	}
	isImage := bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")) ||
		bytes.HasPrefix(head, []byte{0xff, 0xd8}) ||
		bytes.HasPrefix(head, []byte("GIF87a")) || bytes.HasPrefix(head, []byte("GIF89a"))
	var duration float64
	var dimensions mediaProbeDimensions
	var err error
	if isImage {
		var config image.Config
		config, _, err = image.DecodeConfig(&mediaProbeStream{probe: &p})
		if p.failure != nil {
			err = p.failure
		} else if err != nil || config.Width <= 0 || config.Height <= 0 {
			err = errMediaProbeInvalid
		}
		dimensions = mediaProbeDimensions{config.Width, config.Height}
	} else if len(head) >= 8 && mediaProbeBMFFMagic(string(head[4:8])) {
		duration, dimensions, err = p.movie()
	} else {
		return data, []string{"local_media_format_unsupported"}, nil
	}
	if err != nil {
		return mediaProbeFailure(ctx, data, err)
	}
	if err := ctx.Err(); err != nil {
		return data, nil, err
	}
	warnings := []string{}
	sources := data["metadata_sources"].(map[string]string)
	if duration > 0 {
		data["duration_seconds"] = duration
		sources["duration_seconds"] = "mp4_mvhd"
	} else if !isImage {
		warnings = append(warnings, "local_media_duration_unavailable")
	}
	if dimensions.width > 0 && dimensions.height > 0 {
		data["width"], data["height"] = dimensions.width, dimensions.height
		data["resolution"] = fmt.Sprintf("%dx%d", dimensions.width, dimensions.height)
		source := "mp4_tkhd"
		if isImage {
			source = "image_header"
		}
		for _, key := range []string{"width", "height", "resolution"} {
			sources[key] = source
		}
	} else {
		warnings = append(warnings, "local_media_dimensions_unavailable")
	}
	return data, warnings, nil
}

func mediaProbeFailure(ctx context.Context, data map[string]any, err error) (map[string]any, []string, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return data, nil, ctxErr
	}
	switch {
	case errors.Is(err, errMediaProbeInvalid):
		return data, []string{"local_media_metadata_invalid"}, nil
	case errors.Is(err, errMediaProbeLimit):
		return data, []string{"local_media_probe_limit_exceeded"}, nil
	default:
		// Never expose os.PathError, a filename, or decoder error text.
		return data, nil, errMediaProbeRead
	}
}

func mediaProbeBMFFMagic(kind string) bool {
	switch kind {
	case "ftyp", "moov", "mdat", "free", "skip", "wide", "uuid", "styp":
		return true
	}
	return false
}

type mediaProbe struct {
	ctx       context.Context
	file      *os.File
	size      int64
	remaining int64
	boxCount  int
	failure   error
}

func (p *mediaProbe) fail(err error) error {
	p.failure = err
	return err
}

func (p *mediaProbe) readAt(b []byte, offset int64) error {
	if err := p.ctx.Err(); err != nil {
		return p.fail(err)
	}
	if offset < 0 || offset > p.size || int64(len(b)) > p.size-offset {
		return p.fail(errMediaProbeInvalid)
	}
	if int64(len(b)) > p.remaining {
		return p.fail(errMediaProbeLimit)
	}
	p.remaining -= int64(len(b))
	n, err := p.file.ReadAt(b, offset)
	if ctxErr := p.ctx.Err(); ctxErr != nil {
		return p.fail(ctxErr)
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			return p.fail(errMediaProbeInvalid)
		}
		return p.fail(errMediaProbeRead)
	}
	if n != len(b) {
		return p.fail(errMediaProbeInvalid)
	}
	return nil
}

// This reader intentionally provides no Seek method: JPEG APP segments must
// consume the shared byte budget rather than bypass it through seeking.
type mediaProbeStream struct {
	probe  *mediaProbe
	offset int64
}

func (s *mediaProbeStream) Read(b []byte) (int, error) {
	p := s.probe
	if err := p.ctx.Err(); err != nil {
		return 0, p.fail(err)
	}
	if len(b) == 0 {
		return 0, nil
	}
	if s.offset == p.size {
		return 0, io.EOF
	}
	if p.remaining == 0 {
		return 0, p.fail(errMediaProbeLimit)
	}
	n := int(min(int64(len(b)), p.size-s.offset, p.remaining, 32<<10))
	if err := p.readAt(b[:n], s.offset); err != nil {
		return 0, err
	}
	s.offset += int64(n)
	return n, nil
}

type mediaProbeAtom struct {
	kind    string
	payload int64
	end     int64
}

// walk visits only immediate box headers. Unknown payloads, especially mdat,
// are skipped by offset arithmetic, even when they are larger than the budget.
func (p *mediaProbe) walk(start, end int64, depth int, visit func(mediaProbeAtom) error) error {
	if depth > maxMediaProbeDepth {
		return errMediaProbeLimit
	}
	for offset := start; offset < end; {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		if p.boxCount >= maxMediaProbeBoxes {
			return errMediaProbeLimit
		}
		p.boxCount++
		if end-offset < 8 {
			return errMediaProbeInvalid
		}
		var header [16]byte
		if err := p.readAt(header[:8], offset); err != nil {
			return err
		}
		length := uint64(binary.BigEndian.Uint32(header[:4]))
		kind := string(header[4:8])
		headerSize := int64(8)
		switch length {
		case 0:
			length = uint64(end - offset)
		case 1:
			if end-offset < 16 {
				return errMediaProbeInvalid
			}
			if err := p.readAt(header[8:], offset+8); err != nil {
				return err
			}
			length = binary.BigEndian.Uint64(header[8:])
			headerSize = 16
		}
		if kind == "uuid" {
			headerSize += 16
		}
		// Compare before converting length to int64 or adding to offset. This
		// also rejects unsigned overflow and children escaping their parent.
		if length < uint64(headerSize) || length > uint64(end-offset) {
			return errMediaProbeInvalid
		}
		atom := mediaProbeAtom{kind: kind, payload: offset + headerSize, end: offset + int64(length)}
		if err := visit(atom); err != nil {
			return err
		}
		offset = atom.end
	}
	return nil
}

type mediaProbeDimensions struct {
	width  int
	height int
}

func (p *mediaProbe) movie() (float64, mediaProbeDimensions, error) {
	var duration float64
	var dimensions mediaProbeDimensions
	seenMovie, seenHeader := false, false
	err := p.walk(0, p.size, 1, func(atom mediaProbeAtom) error {
		switch atom.kind {
		case "ftyp", "styp":
			if atom.end-atom.payload < 8 || (atom.end-atom.payload)%4 != 0 {
				return errMediaProbeInvalid
			}
		case "moov":
			if seenMovie {
				return errMediaProbeInvalid
			}
			seenMovie = true
			return p.walk(atom.payload, atom.end, 2, func(child mediaProbeAtom) error {
				switch child.kind {
				case "mvhd":
					if seenHeader {
						return errMediaProbeInvalid
					}
					seenHeader = true
					var err error
					duration, err = p.movieDuration(child)
					return err
				case "trak":
					trackDimensions, err := p.track(child)
					// Choose the first video track with usable dimensions;
					// audio and untyped tracks can never supply them.
					if dimensions.width == 0 && trackDimensions.width > 0 {
						dimensions = trackDimensions
					}
					return err
				}
				return nil
			})
		}
		return nil
	})
	return duration, dimensions, err
}

// fullHeader reads only a fixed-size prefix (at most 112 bytes), never a
// declared box-sized allocation. Unrecognized versions leave fields unknown.
func (p *mediaProbe) fullHeader(atom mediaProbeAtom, version0Size, version1Size int) ([]byte, error) {
	if atom.end-atom.payload < 4 {
		return nil, errMediaProbeInvalid
	}
	var version [4]byte
	if err := p.readAt(version[:], atom.payload); err != nil {
		return nil, err
	}
	n := version0Size
	switch version[0] {
	case 0:
	case 1:
		n = version1Size
	default:
		return nil, nil
	}
	if atom.end-atom.payload < int64(n) {
		return nil, errMediaProbeInvalid
	}
	b := make([]byte, n)
	if err := p.readAt(b, atom.payload); err != nil {
		return nil, err
	}
	return b, nil
}

func (p *mediaProbe) movieDuration(atom mediaProbeAtom) (float64, error) {
	b, err := p.fullHeader(atom, 100, 112)
	if err != nil || b == nil {
		return 0, err
	}
	var scale uint32
	var duration uint64
	if b[0] == 0 {
		scale = binary.BigEndian.Uint32(b[12:16])
		duration = uint64(binary.BigEndian.Uint32(b[16:20]))
		if duration == math.MaxUint32 {
			return 0, nil
		}
	} else {
		scale = binary.BigEndian.Uint32(b[20:24])
		duration = binary.BigEndian.Uint64(b[24:32])
		if duration == math.MaxUint64 {
			return 0, nil
		}
	}
	if scale == 0 || duration == 0 {
		return 0, nil
	}
	return float64(duration) / float64(scale), nil
}

func (p *mediaProbe) track(atom mediaProbeAtom) (mediaProbeDimensions, error) {
	var dimensions mediaProbeDimensions
	var handler string
	seenHeader, seenMedia, seenHandler := false, false, false
	err := p.walk(atom.payload, atom.end, 3, func(child mediaProbeAtom) error {
		switch child.kind {
		case "tkhd":
			if seenHeader {
				return errMediaProbeInvalid
			}
			seenHeader = true
			var err error
			dimensions, err = p.trackDimensions(child)
			return err
		case "mdia":
			if seenMedia {
				return errMediaProbeInvalid
			}
			seenMedia = true
			return p.walk(child.payload, child.end, 4, func(media mediaProbeAtom) error {
				if media.kind != "hdlr" {
					return nil
				}
				if seenHandler || media.end-media.payload < 24 {
					return errMediaProbeInvalid
				}
				seenHandler = true
				var b [24]byte
				if err := p.readAt(b[:], media.payload); err != nil {
					return err
				}
				if b[0] != 0 {
					return errMediaProbeInvalid
				}
				handler = string(b[8:12])
				return nil
			})
		}
		return nil
	})
	if err != nil || handler != "vide" {
		return mediaProbeDimensions{}, err
	}
	return dimensions, nil
}

func (p *mediaProbe) trackDimensions(atom mediaProbeAtom) (mediaProbeDimensions, error) {
	b, err := p.fullHeader(atom, 84, 96)
	if err != nil || b == nil {
		return mediaProbeDimensions{}, err
	}
	matrixOffset, dimensionsOffset := 40, 76
	if b[0] == 1 {
		matrixOffset, dimensionsOffset = 52, 88
	}
	width := int(binary.BigEndian.Uint32(b[dimensionsOffset:]) >> 16)
	height := int(binary.BigEndian.Uint32(b[dimensionsOffset+4:]) >> 16)
	if width == 0 || height == 0 {
		return mediaProbeDimensions{}, nil
	}
	matrix := b[matrixOffset:]
	a := int32(binary.BigEndian.Uint32(matrix[0:4]))
	bValue := int32(binary.BigEndian.Uint32(matrix[4:8]))
	c := int32(binary.BigEndian.Uint32(matrix[12:16]))
	d := int32(binary.BigEndian.Uint32(matrix[16:20]))
	// Canonical quarter-turn matrices use signed 16.16 values. Translation
	// does not affect dimensions. Other affine transforms are not interpreted.
	if a == 0 && d == 0 && ((bValue == 1<<16 && c == -1<<16) || (bValue == -1<<16 && c == 1<<16)) {
		width, height = height, width
	}
	return mediaProbeDimensions{width, height}, nil
}
