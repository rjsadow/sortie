package recordings

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
)

const (
	vrecMagic      uint32 = 0x56524543 // "VREC"
	vrecHeaderSize int    = 12
)

const (
	// VNC server→client message types
	vncFramebufferUpdate   byte = 0
	vncSetColourMapEntries byte = 1
	vncBell                byte = 2
	vncServerCutText       byte = 3
)

const (
	// VNC encoding types
	vncEncodingRaw             int32 = 0
	vncEncodingDesktopSize     int32 = -223
	vncEncodingLastRect        int32 = -224
	vncEncodingExtendedDesktop int32 = -308
)

const (
	// Default conversion settings
	defaultFPS int = 10
)

// VRECHeader is the header of a .vncrec file.
type VRECHeader struct {
	Width  int
	Height int
}

// RecordedFrame is a single captured WebSocket message from a .vncrec recording.
type RecordedFrame struct {
	FromClient bool
	Timestamp  uint32 // ms since recording start
	Data       []byte
}

// ParseVRECFile reads a .vncrec file and returns the header and frames.
func ParseVRECFile(path string) (*VRECHeader, []RecordedFrame, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read file: %w", err)
	}
	return ParseVREC(data)
}

// ParseVREC parses the in-memory content of a .vncrec recording.
func ParseVREC(data []byte) (*VRECHeader, []RecordedFrame, error) {
	var header *VRECHeader
	offset := 0

	// Parse VREC header if present
	if len(data) >= vrecHeaderSize {
		magic := binary.BigEndian.Uint32(data[0:4])
		if magic == vrecMagic {
			version := binary.BigEndian.Uint16(data[4:6])
			if version == 1 {
				header = &VRECHeader{
					Width:  int(binary.BigEndian.Uint16(data[6:8])),
					Height: int(binary.BigEndian.Uint16(data[8:10])),
				}
				offset = vrecHeaderSize
			}
		}
	}

	if header == nil {
		header = &VRECHeader{Width: 1024, Height: 768}
	}

	// Parse binary frames
	var frames []RecordedFrame
	for offset+9 <= len(data) {
		fromClient := data[offset] == 1
		timestamp := binary.BigEndian.Uint32(data[offset+1 : offset+5])
		dataLen := binary.BigEndian.Uint32(data[offset+5 : offset+9])
		offset += 9

		if offset+int(dataLen) > len(data) {
			break
		}

		frameData := make([]byte, dataLen)
		copy(frameData, data[offset:offset+int(dataLen)])
		frames = append(frames, RecordedFrame{
			FromClient: fromClient,
			Timestamp:  timestamp,
			Data:       frameData,
		})
		offset += int(dataLen)
	}

	return header, frames, nil
}

// pixelFormat describes VNC pixel encoding.
type pixelFormat struct {
	bitsPerPixel uint8
	depth        uint8
	bigEndian    bool
	trueColour   bool
	redMax       uint16
	greenMax     uint16
	blueMax      uint16
	redShift     uint8
	greenShift   uint8
	blueShift    uint8
}

// framebuffer holds the current screen pixel state.
type framebuffer struct {
	width      int
	height     int
	pixels     []byte // BGRA, 4 bytes per pixel
	bpp        int    // bytes per pixel from VNC (default 4)
	redShift   uint8
	greenShift uint8
	blueShift  uint8
}

func newFramebuffer(width, height int) *framebuffer {
	return &framebuffer{
		width:      width,
		height:     height,
		pixels:     make([]byte, width*height*4),
		bpp:        4,
		redShift:   16,
		greenShift: 8,
		blueShift:  0,
	}
}

func (fb *framebuffer) setPixelFormat(pf *pixelFormat) {
	fb.bpp = int(pf.bitsPerPixel) / 8
	if fb.bpp < 1 {
		fb.bpp = 4
	}
	fb.redShift = pf.redShift
	fb.greenShift = pf.greenShift
	fb.blueShift = pf.blueShift
}

// writeRawRect writes a Raw-encoded rectangle to the framebuffer.
func (fb *framebuffer) writeRawRect(x, y, w, h int, data []byte) {
	srcBpp := fb.bpp
	for row := range h {
		dy := y + row
		if dy < 0 || dy >= fb.height {
			continue
		}
		for col := range w {
			dx := x + col
			if dx < 0 || dx >= fb.width {
				continue
			}

			srcOff := (row*w + col) * srcBpp
			if srcOff+srcBpp > len(data) {
				return
			}

			// Read pixel value (little-endian)
			var pixel uint32
			switch srcBpp {
			case 4:
				pixel = uint32(data[srcOff]) |
					uint32(data[srcOff+1])<<8 |
					uint32(data[srcOff+2])<<16 |
					uint32(data[srcOff+3])<<24
			case 2:
				pixel = uint32(data[srcOff]) | uint32(data[srcOff+1])<<8
			default:
				pixel = uint32(data[srcOff])
			}

			// Extract RGB using VNC pixel format shifts
			r := uint8((pixel >> fb.redShift) & 0xFF)
			g := uint8((pixel >> fb.greenShift) & 0xFF)
			b := uint8((pixel >> fb.blueShift) & 0xFF)

			// Write to framebuffer as BGRA (for ffmpeg bgra input)
			dstOff := (dy*fb.width + dx) * 4
			fb.pixels[dstOff] = b
			fb.pixels[dstOff+1] = g
			fb.pixels[dstOff+2] = r
			fb.pixels[dstOff+3] = 255
		}
	}
}

// resize changes the framebuffer dimensions, discarding current content.
func (fb *framebuffer) resize(width, height int) {
	fb.width = width
	fb.height = height
	fb.pixels = make([]byte, width*height*4)
}

// tsEntry maps a byte offset in the concatenated VNC stream to a frame timestamp.
type tsEntry struct {
	offset    int
	timestamp uint32
}

// ConvertToMP4 converts a .vncrec recording to an MP4 video file using ffmpeg.
func ConvertToMP4(inputPath, outputPath string) error {
	header, frames, err := ParseVRECFile(inputPath)
	if err != nil {
		return fmt.Errorf("parse vncrec: %w", err)
	}

	if len(frames) == 0 {
		return fmt.Errorf("recording contains no frames")
	}

	width := header.Width
	height := header.Height

	// Collect server→client frames only
	var serverFrames []RecordedFrame
	for _, f := range frames {
		if !f.FromClient {
			serverFrames = append(serverFrames, f)
		}
	}

	if len(serverFrames) == 0 {
		return fmt.Errorf("recording contains no server frames")
	}

	// Concatenate server data into a continuous VNC stream.
	// Also build a timestamp index mapping byte offsets to frame timestamps.
	var stream []byte
	var tsMap []tsEntry
	for _, f := range serverFrames {
		tsMap = append(tsMap, tsEntry{offset: len(stream), timestamp: f.Timestamp})
		stream = append(stream, f.Data...)
	}

	// Parse VNC protocol
	fb := newFramebuffer(width, height)
	pos := 0

	// Check if stream starts with VNC handshake ("RFB ")
	if len(stream) >= 4 && stream[0] == 'R' && stream[1] == 'F' && stream[2] == 'B' && stream[3] == ' ' {
		newPos, pf, err := parseVNCHandshake(stream)
		if err != nil {
			slog.Warn("VNC handshake parse failed, attempting without", "error", err)
		} else {
			pos = newPos
			if pf != nil {
				fb.setPixelFormat(pf)
			}
		}
	}

	// H.264 with yuv420p requires even dimensions. Use a filter to pad if needed.
	var vfFilter string
	if width%2 != 0 || height%2 != 0 {
		// Pad to next even dimensions (adds a 1px black border on right/bottom if needed)
		padW := width + width%2
		padH := height + height%2
		vfFilter = fmt.Sprintf("pad=%d:%d:0:0:black", padW, padH)
	}

	// Start ffmpeg process
	ffmpegArgs := []string{
		"-y",                    // overwrite output
		"-f", "rawvideo",        // input format
		"-pix_fmt", "bgra",      // pixel format
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-r", fmt.Sprintf("%d", defaultFPS),
		"-i", "pipe:0",          // read from stdin
	}
	if vfFilter != "" {
		ffmpegArgs = append(ffmpegArgs, "-vf", vfFilter)
	}
	ffmpegArgs = append(ffmpegArgs,
		"-c:v", "libx264",       // H.264 codec
		"-pix_fmt", "yuv420p",   // output pixel format (wide compatibility)
		"-crf", "23",            // quality (lower = better)
		"-preset", "fast",       // encoding speed
		"-movflags", "+faststart", // enable streaming
		outputPath,
	)
	cmd := exec.Command("ffmpeg", ffmpegArgs...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdin pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Process VNC stream and emit video frames at fixed intervals
	frameIntervalMs := uint32(1000 / defaultFPS)

	// Look up the timestamp for a given byte position in the VNC stream
	getTimestamp := func(bytePos int) uint32 {
		var ts uint32
		for _, entry := range tsMap {
			if entry.offset <= bytePos {
				ts = entry.timestamp
			} else {
				break
			}
		}
		return ts
	}

	var nextEmitTime uint32
	framesWritten := 0

	for pos < len(stream) {
		currentTs := getTimestamp(pos)

		// Emit video frames for all intervals up to the current VNC message timestamp
		for nextEmitTime <= currentTs {
			if _, werr := stdin.Write(fb.pixels); werr != nil {
				slog.Warn("ffmpeg write error", "error", werr)
				goto done
			}
			framesWritten++
			nextEmitTime += frameIntervalMs
		}

		// Process the next VNC server→client message
		newPos, perr := processVNCMessage(stream, pos, fb)
		if perr != nil {
			slog.Debug("VNC parse stopped", "error", perr, "offset", pos, "total", len(stream))
			break
		}
		pos = newPos
	}

done:
	// Emit at least one final frame so the video isn't empty
	if framesWritten == 0 || pos >= len(stream) {
		stdin.Write(fb.pixels)
	}

	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg exited with error: %w; stderr: %s", err, stderrBuf.String())
	}

	return nil
}

// parseVNCHandshake parses the server→client portion of the VNC handshake.
// Returns the byte offset after ServerInit and the pixel format from ServerInit.
func parseVNCHandshake(stream []byte) (int, *pixelFormat, error) {
	pos := 0

	// 1. ProtocolVersion: "RFB xxx.yyy\n" (12 bytes)
	if pos+12 > len(stream) {
		return 0, nil, fmt.Errorf("too short for ProtocolVersion")
	}
	pos += 12

	// 2. SecurityTypes: [count, type1, type2, ...]
	if pos+1 > len(stream) {
		return 0, nil, fmt.Errorf("too short for SecurityTypes count")
	}
	count := int(stream[pos])
	pos++

	if count == 0 {
		// Failure case: 4 bytes reason length + reason string
		if pos+4 > len(stream) {
			return 0, nil, fmt.Errorf("security failure, truncated reason")
		}
		reasonLen := int(binary.BigEndian.Uint32(stream[pos : pos+4]))
		return 0, nil, fmt.Errorf("VNC security failure: %s", string(stream[pos+4:pos+4+reasonLen]))
	}

	// Check for VNC Auth (type 2) which adds 16 bytes challenge
	hasVNCAuth := false
	for i := 0; i < count && pos+i < len(stream); i++ {
		if stream[pos+i] == 2 {
			hasVNCAuth = true
		}
	}
	pos += count

	// For VNC Auth, server sends 16-byte challenge
	if hasVNCAuth {
		if pos+16 > len(stream) {
			return 0, nil, fmt.Errorf("too short for VNC Auth challenge")
		}
		pos += 16
	}

	// 3. SecurityResult: 4 bytes uint32 BE
	if pos+4 > len(stream) {
		return 0, nil, fmt.Errorf("too short for SecurityResult")
	}
	secResult := binary.BigEndian.Uint32(stream[pos : pos+4])
	pos += 4
	if secResult != 0 {
		return 0, nil, fmt.Errorf("VNC SecurityResult failure: %d", secResult)
	}

	// 4. ServerInit: width(2) + height(2) + pixelFormat(16) + nameLen(4) + name
	if pos+24 > len(stream) {
		return 0, nil, fmt.Errorf("too short for ServerInit")
	}

	pfOff := pos + 4 // skip width(2) + height(2)
	pf := &pixelFormat{
		bitsPerPixel: stream[pfOff],
		depth:        stream[pfOff+1],
		bigEndian:    stream[pfOff+2] != 0,
		trueColour:   stream[pfOff+3] != 0,
		redMax:       binary.BigEndian.Uint16(stream[pfOff+4 : pfOff+6]),
		greenMax:     binary.BigEndian.Uint16(stream[pfOff+6 : pfOff+8]),
		blueMax:      binary.BigEndian.Uint16(stream[pfOff+8 : pfOff+10]),
		redShift:     stream[pfOff+10],
		greenShift:   stream[pfOff+11],
		blueShift:    stream[pfOff+12],
	}

	nameLen := int(binary.BigEndian.Uint32(stream[pos+20 : pos+24]))
	pos += 24
	if pos+nameLen > len(stream) {
		return 0, nil, fmt.Errorf("too short for server name (need %d more bytes)", nameLen-(len(stream)-pos))
	}
	pos += nameLen

	return pos, pf, nil
}

// processVNCMessage processes one VNC server→client message starting at pos.
// It updates the framebuffer for FramebufferUpdate messages.
// Returns the new stream position after the message.
func processVNCMessage(stream []byte, pos int, fb *framebuffer) (int, error) {
	if pos >= len(stream) {
		return pos, fmt.Errorf("end of stream")
	}

	msgType := stream[pos]

	switch msgType {
	case vncFramebufferUpdate:
		return processFramebufferUpdate(stream, pos, fb)
	case vncSetColourMapEntries:
		return skipSetColourMapEntries(stream, pos)
	case vncBell:
		return pos + 1, nil
	case vncServerCutText:
		return skipServerCutText(stream, pos)
	default:
		return pos, fmt.Errorf("unknown VNC message type %d at offset %d", msgType, pos)
	}
}

// processFramebufferUpdate handles a FramebufferUpdate (type 0) message.
func processFramebufferUpdate(stream []byte, pos int, fb *framebuffer) (int, error) {
	// Header: type(1) + padding(1) + numRects(2)
	if pos+4 > len(stream) {
		return pos, fmt.Errorf("truncated FramebufferUpdate header")
	}
	numRects := int(binary.BigEndian.Uint16(stream[pos+2 : pos+4]))
	pos += 4

	for i := range numRects {
		if pos+12 > len(stream) {
			return pos, fmt.Errorf("truncated rectangle header %d/%d", i+1, numRects)
		}

		x := int(binary.BigEndian.Uint16(stream[pos : pos+2]))
		y := int(binary.BigEndian.Uint16(stream[pos+2 : pos+4]))
		w := int(binary.BigEndian.Uint16(stream[pos+4 : pos+6]))
		h := int(binary.BigEndian.Uint16(stream[pos+6 : pos+8]))
		encoding := int32(binary.BigEndian.Uint32(stream[pos+8 : pos+12]))
		pos += 12

		switch encoding {
		case vncEncodingRaw:
			pixelBytes := w * h * fb.bpp
			if pos+pixelBytes > len(stream) {
				return pos, fmt.Errorf("truncated raw pixel data: need %d bytes, have %d", pixelBytes, len(stream)-pos)
			}
			fb.writeRawRect(x, y, w, h, stream[pos:pos+pixelBytes])
			pos += pixelBytes

		case vncEncodingDesktopSize:
			// Pseudo-encoding: resize desktop. No pixel data follows.
			fb.resize(w, h)

		case vncEncodingLastRect:
			// Pseudo-encoding: marks end of rectangles. No data.
			return pos, nil

		case vncEncodingExtendedDesktop:
			// ExtendedDesktopSize: 4 bytes (num screens) + screens data
			if pos+4 > len(stream) {
				return pos, fmt.Errorf("truncated ExtendedDesktopSize")
			}
			numScreens := int(stream[pos])
			// Skip: 1 (numScreens) + 3 (padding) + numScreens * 16
			skip := 4 + numScreens*16
			if pos+skip > len(stream) {
				return pos, fmt.Errorf("truncated ExtendedDesktopSize screens")
			}
			fb.resize(w, h)
			pos += skip

		default:
			// Unsupported encoding (Tight, ZRLE, etc.) — we can't determine the
			// data length so we must stop processing this FramebufferUpdate.
			// Return current position without error so the main loop can continue
			// looking for the next VNC message.
			slog.Debug("Skipping unsupported encoding", "encoding", encoding, "rect", i+1, "of", numRects)
			return pos, nil
		}
	}

	return pos, nil
}

// skipSetColourMapEntries skips a SetColourMapEntries (type 1) message.
func skipSetColourMapEntries(stream []byte, pos int) (int, error) {
	// type(1) + padding(1) + firstColour(2) + numColours(2) + colours(numColours*6)
	if pos+6 > len(stream) {
		return pos, fmt.Errorf("truncated SetColourMapEntries")
	}
	numColours := int(binary.BigEndian.Uint16(stream[pos+4 : pos+6]))
	total := 6 + numColours*6
	if pos+total > len(stream) {
		return pos, fmt.Errorf("truncated SetColourMapEntries colour data")
	}
	return pos + total, nil
}

// skipServerCutText skips a ServerCutText (type 3) message.
func skipServerCutText(stream []byte, pos int) (int, error) {
	// type(1) + padding(3) + length(4) + text(length)
	if pos+8 > len(stream) {
		return pos, fmt.Errorf("truncated ServerCutText")
	}
	textLen := int(binary.BigEndian.Uint32(stream[pos+4 : pos+8]))
	total := 8 + textLen
	if pos+total > len(stream) {
		return pos, fmt.Errorf("truncated ServerCutText text data")
	}
	return pos + total, nil
}
