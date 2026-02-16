package recordings

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildVRECTestHeader creates a 12-byte VREC header for testing.
func buildVRECTestHeader(width, height uint16) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[0:4], uint32(vrecMagic))
	binary.BigEndian.PutUint16(buf[4:6], 1)      // version
	binary.BigEndian.PutUint16(buf[6:8], width)   // width
	binary.BigEndian.PutUint16(buf[8:10], height)  // height
	binary.BigEndian.PutUint16(buf[10:12], 0)     // reserved
	return buf
}

// buildTestFrame creates a binary recorded frame.
func buildTestFrame(fromClient bool, timestampMs uint32, data []byte) []byte {
	buf := make([]byte, 9+len(data))
	if fromClient {
		buf[0] = 1
	}
	binary.BigEndian.PutUint32(buf[1:5], timestampMs)
	binary.BigEndian.PutUint32(buf[5:9], uint32(len(data)))
	copy(buf[9:], data)
	return buf
}

// buildVNCHandshakeServer builds the server→client VNC handshake for testing.
// Returns: ProtocolVersion + SecurityTypes(None) + SecurityResult(OK) + ServerInit.
func buildVNCHandshakeServer(width, height uint16) []byte {
	name := []byte("TestVNC")
	total := 12 + 2 + 4 + 24 + len(name)
	buf := make([]byte, total)
	off := 0

	// ProtocolVersion
	copy(buf[off:off+12], "RFB 003.008\n")
	off += 12

	// SecurityTypes: 1 type, type=None(1)
	buf[off] = 1 // count
	buf[off+1] = 1 // None
	off += 2

	// SecurityResult: OK (0)
	binary.BigEndian.PutUint32(buf[off:off+4], 0)
	off += 4

	// ServerInit: width + height + pixel format + name
	binary.BigEndian.PutUint16(buf[off:off+2], width)
	binary.BigEndian.PutUint16(buf[off+2:off+4], height)
	off += 4

	// Pixel format (16 bytes) — 32bpp little-endian RGBX
	buf[off] = 32  // bits-per-pixel
	buf[off+1] = 24 // depth
	buf[off+2] = 0  // big-endian-flag: false
	buf[off+3] = 1  // true-colour-flag: true
	binary.BigEndian.PutUint16(buf[off+4:off+6], 255)  // red-max
	binary.BigEndian.PutUint16(buf[off+6:off+8], 255)  // green-max
	binary.BigEndian.PutUint16(buf[off+8:off+10], 255) // blue-max
	buf[off+10] = 16 // red-shift
	buf[off+11] = 8  // green-shift
	buf[off+12] = 0  // blue-shift
	// bytes 13-15 = padding (0)
	off += 16

	// Name length + name
	binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(name)))
	off += 4
	copy(buf[off:], name)

	return buf
}

// buildRawFBUpdate builds a FramebufferUpdate message with one Raw rectangle.
// Pixel data is filled with the given BGRX color.
func buildRawFBUpdate(x, y, w, h uint16, r, g, b byte) []byte {
	pixelCount := int(w) * int(h)
	pixels := make([]byte, pixelCount*4)
	for i := range pixelCount {
		pixels[i*4] = b   // blue at byte 0 (shift 0)
		pixels[i*4+1] = g // green at byte 1 (shift 8)
		pixels[i*4+2] = r // red at byte 2 (shift 16)
		pixels[i*4+3] = 0 // padding
	}

	// Header: type(1) + padding(1) + numRects(2) + rect header(12)
	buf := make([]byte, 4+12+len(pixels))
	buf[0] = 0 // FramebufferUpdate
	buf[1] = 0 // padding
	binary.BigEndian.PutUint16(buf[2:4], 1) // 1 rectangle

	binary.BigEndian.PutUint16(buf[4:6], x)
	binary.BigEndian.PutUint16(buf[6:8], y)
	binary.BigEndian.PutUint16(buf[8:10], w)
	binary.BigEndian.PutUint16(buf[10:12], h)
	binary.BigEndian.PutUint32(buf[12:16], 0) // Raw encoding

	copy(buf[16:], pixels)
	return buf
}

func TestParseVREC_WithHeader(t *testing.T) {
	var data []byte
	data = append(data, buildVRECTestHeader(800, 600)...)
	data = append(data, buildTestFrame(false, 0, []byte{1, 2, 3})...)
	data = append(data, buildTestFrame(true, 100, []byte{4, 5})...)
	data = append(data, buildTestFrame(false, 200, []byte{6})...)

	header, frames, err := ParseVREC(data)
	if err != nil {
		t.Fatalf("ParseVREC() error = %v", err)
	}

	if header.Width != 800 || header.Height != 600 {
		t.Errorf("header = %+v, want 800x600", header)
	}

	if len(frames) != 3 {
		t.Fatalf("len(frames) = %d, want 3", len(frames))
	}

	if frames[0].FromClient != false || frames[0].Timestamp != 0 || len(frames[0].Data) != 3 {
		t.Errorf("frame[0] = %+v", frames[0])
	}
	if frames[1].FromClient != true || frames[1].Timestamp != 100 || len(frames[1].Data) != 2 {
		t.Errorf("frame[1] = %+v", frames[1])
	}
	if frames[2].FromClient != false || frames[2].Timestamp != 200 || len(frames[2].Data) != 1 {
		t.Errorf("frame[2] = %+v", frames[2])
	}
}

func TestParseVREC_WithoutHeader(t *testing.T) {
	// Data without VREC magic → defaults to 1024x768
	var data []byte
	data = append(data, buildTestFrame(false, 0, []byte{0xAB})...)

	header, frames, err := ParseVREC(data)
	if err != nil {
		t.Fatalf("ParseVREC() error = %v", err)
	}

	if header.Width != 1024 || header.Height != 768 {
		t.Errorf("header = %+v, want 1024x768 defaults", header)
	}
	if len(frames) != 1 {
		t.Fatalf("len(frames) = %d, want 1", len(frames))
	}
}

func TestParseVREC_Empty(t *testing.T) {
	header, frames, err := ParseVREC([]byte{})
	if err != nil {
		t.Fatalf("ParseVREC() error = %v", err)
	}
	if header.Width != 1024 || header.Height != 768 {
		t.Errorf("header = %+v, want defaults", header)
	}
	if len(frames) != 0 {
		t.Errorf("len(frames) = %d, want 0", len(frames))
	}
}

func TestParseVREC_TruncatedFrame(t *testing.T) {
	var data []byte
	data = append(data, buildVRECTestHeader(640, 480)...)
	// Add a frame that claims 100 bytes of data but only has 5
	frame := make([]byte, 9+5)
	frame[0] = 0 // server
	binary.BigEndian.PutUint32(frame[1:5], 0)
	binary.BigEndian.PutUint32(frame[5:9], 100) // claims 100 bytes
	copy(frame[9:], []byte{1, 2, 3, 4, 5})
	data = append(data, frame...)

	_, frames, err := ParseVREC(data)
	if err != nil {
		t.Fatalf("ParseVREC() error = %v", err)
	}
	// Should skip the truncated frame
	if len(frames) != 0 {
		t.Errorf("len(frames) = %d, want 0 (truncated frame should be skipped)", len(frames))
	}
}

func TestParseVNCHandshake(t *testing.T) {
	stream := buildVNCHandshakeServer(1024, 768)

	pos, pf, err := parseVNCHandshake(stream)
	if err != nil {
		t.Fatalf("parseVNCHandshake() error = %v", err)
	}

	if pos != len(stream) {
		t.Errorf("pos = %d, want %d", pos, len(stream))
	}

	if pf == nil {
		t.Fatal("pixel format is nil")
	}
	if pf.bitsPerPixel != 32 {
		t.Errorf("bitsPerPixel = %d, want 32", pf.bitsPerPixel)
	}
	if pf.redShift != 16 {
		t.Errorf("redShift = %d, want 16", pf.redShift)
	}
	if pf.greenShift != 8 {
		t.Errorf("greenShift = %d, want 8", pf.greenShift)
	}
	if pf.blueShift != 0 {
		t.Errorf("blueShift = %d, want 0", pf.blueShift)
	}
}

func TestFramebuffer_WriteRawRect(t *testing.T) {
	fb := newFramebuffer(4, 4)

	// Write a 2x2 red rectangle at (1, 1)
	// Pixel format: little-endian 32bpp, red-shift=16, green-shift=8, blue-shift=0
	// Red pixel: B=0, G=0, R=255, X=0 → 0x00FF0000 in LE → [0x00, 0x00, 0xFF, 0x00]
	pixels := make([]byte, 2*2*4)
	for i := range 4 {
		pixels[i*4] = 0    // blue
		pixels[i*4+1] = 0  // green
		pixels[i*4+2] = 255 // red
		pixels[i*4+3] = 0  // padding
	}

	fb.writeRawRect(1, 1, 2, 2, pixels)

	// Check pixel at (1,1) — should be red (in BGRA output: B=0, G=0, R=255, A=255)
	off := (1*4 + 1) * 4
	if fb.pixels[off] != 0 || fb.pixels[off+1] != 0 || fb.pixels[off+2] != 255 || fb.pixels[off+3] != 255 {
		t.Errorf("pixel at (1,1) = [%d,%d,%d,%d], want [0,0,255,255]",
			fb.pixels[off], fb.pixels[off+1], fb.pixels[off+2], fb.pixels[off+3])
	}

	// Check pixel at (0,0) — should be black (default)
	off = 0
	if fb.pixels[off] != 0 || fb.pixels[off+1] != 0 || fb.pixels[off+2] != 0 || fb.pixels[off+3] != 0 {
		t.Errorf("pixel at (0,0) = [%d,%d,%d,%d], want [0,0,0,0]",
			fb.pixels[off], fb.pixels[off+1], fb.pixels[off+2], fb.pixels[off+3])
	}
}

func TestFramebuffer_WriteRawRect_OutOfBounds(t *testing.T) {
	fb := newFramebuffer(4, 4)

	// Write a rect that extends beyond the framebuffer
	pixels := make([]byte, 3*3*4) // 3x3 at position (2,2) → extends to (4,4) which is out of bounds
	for i := range pixels {
		pixels[i] = 0xFF
	}

	// Should not panic
	fb.writeRawRect(2, 2, 3, 3, pixels)
}

func TestProcessVNCMessage_FramebufferUpdate(t *testing.T) {
	fb := newFramebuffer(10, 10)
	msg := buildRawFBUpdate(0, 0, 10, 10, 128, 64, 32) // fill entire screen

	pos, err := processVNCMessage(msg, 0, fb)
	if err != nil {
		t.Fatalf("processVNCMessage() error = %v", err)
	}
	if pos != len(msg) {
		t.Errorf("pos = %d, want %d", pos, len(msg))
	}

	// Check pixel (0,0) is the expected color (BGRA output)
	if fb.pixels[0] != 32 || fb.pixels[1] != 64 || fb.pixels[2] != 128 || fb.pixels[3] != 255 {
		t.Errorf("pixel at (0,0) = [%d,%d,%d,%d], want [32,64,128,255]",
			fb.pixels[0], fb.pixels[1], fb.pixels[2], fb.pixels[3])
	}
}

func TestProcessVNCMessage_Bell(t *testing.T) {
	fb := newFramebuffer(4, 4)
	msg := []byte{2} // Bell message (type 2)

	pos, err := processVNCMessage(msg, 0, fb)
	if err != nil {
		t.Fatalf("processVNCMessage() error = %v", err)
	}
	if pos != 1 {
		t.Errorf("pos = %d, want 1", pos)
	}
}

func TestProcessVNCMessage_ServerCutText(t *testing.T) {
	fb := newFramebuffer(4, 4)
	text := []byte("hello")
	msg := make([]byte, 8+len(text))
	msg[0] = 3 // ServerCutText
	binary.BigEndian.PutUint32(msg[4:8], uint32(len(text)))
	copy(msg[8:], text)

	pos, err := processVNCMessage(msg, 0, fb)
	if err != nil {
		t.Fatalf("processVNCMessage() error = %v", err)
	}
	if pos != len(msg) {
		t.Errorf("pos = %d, want %d", pos, len(msg))
	}
}

func TestProcessVNCMessage_UnknownType(t *testing.T) {
	fb := newFramebuffer(4, 4)
	msg := []byte{99} // Unknown type

	_, err := processVNCMessage(msg, 0, fb)
	if err == nil {
		t.Fatal("expected error for unknown message type")
	}
}

func TestConvertToMP4_EndToEnd(t *testing.T) {
	// Skip if ffmpeg is not available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "test.vncrec")
	outputPath := filepath.Join(tmpDir, "test.mp4")

	// Build a test .vncrec file:
	// VREC header + VNC handshake + two FramebufferUpdates (red then blue)
	var data []byte
	data = append(data, buildVRECTestHeader(64, 48)...)

	handshake := buildVNCHandshakeServer(64, 48)
	data = append(data, buildTestFrame(false, 0, handshake)...)

	redFrame := buildRawFBUpdate(0, 0, 64, 48, 255, 0, 0)
	data = append(data, buildTestFrame(false, 100, redFrame)...)

	blueFrame := buildRawFBUpdate(0, 0, 64, 48, 0, 0, 255)
	data = append(data, buildTestFrame(false, 500, blueFrame)...)

	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err := ConvertToMP4(inputPath, outputPath)
	if err != nil {
		t.Fatalf("ConvertToMP4() error = %v", err)
	}

	// Verify output exists and is non-empty
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
	t.Logf("Output file size: %d bytes", info.Size())
}

func TestConvertToMP4_NoFrames(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "empty.vncrec")
	outputPath := filepath.Join(tmpDir, "empty.mp4")

	// VREC header only, no frames
	data := buildVRECTestHeader(64, 48)
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err := ConvertToMP4(inputPath, outputPath)
	if err == nil {
		t.Fatal("expected error for recording with no frames")
	}
}

func TestConvertToMP4_WithoutHandshake(t *testing.T) {
	// Test conversion of a recording that starts directly with FramebufferUpdate
	// (legacy recording or one that started after connection)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "nohandshake.vncrec")
	outputPath := filepath.Join(tmpDir, "nohandshake.mp4")

	var data []byte
	data = append(data, buildVRECTestHeader(32, 24)...)

	greenFrame := buildRawFBUpdate(0, 0, 32, 24, 0, 255, 0)
	data = append(data, buildTestFrame(false, 0, greenFrame)...)
	data = append(data, buildTestFrame(false, 500, greenFrame)...)

	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err := ConvertToMP4(inputPath, outputPath)
	if err != nil {
		t.Fatalf("ConvertToMP4() error = %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
}
