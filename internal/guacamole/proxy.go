package guacamole

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// GuacdProxy manages a connection between a WebSocket client and guacd.
// It performs the Guacamole protocol handshake and bidirectionally relays
// Guacamole instructions between the client and guacd.
type GuacdProxy struct {
	guacdAddr string // e.g. "10.0.0.5:4822"
	hostname  string // RDP hostname (usually "localhost" since containers share network)
	port      string // RDP port (usually "3389")
	username  string // RDP username
	password  string // RDP password
	width     string // Screen width in pixels
	height    string // Screen height in pixels
}

// NewGuacdProxy creates a new proxy that will connect to guacd at the given address
// and instruct it to connect to the RDP server at hostname:port.
func NewGuacdProxy(guacdAddr, hostname, port, username, password, width, height string) *GuacdProxy {
	return &GuacdProxy{
		guacdAddr: guacdAddr,
		hostname:  hostname,
		port:      port,
		username:  username,
		password:  password,
		width:     width,
		height:    height,
	}
}

// Serve connects to guacd, performs the Guacamole handshake for RDP,
// and then bidirectionally relays data between the WebSocket client and guacd.
func (p *GuacdProxy) Serve(clientConn *websocket.Conn) error {
	// Connect to guacd via TCP
	guacdConn, err := net.Dial("tcp", p.guacdAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to guacd at %s: %w", p.guacdAddr, err)
	}
	defer guacdConn.Close()

	// Perform the Guacamole handshake
	if err := p.handshake(guacdConn); err != nil {
		return fmt.Errorf("guacamole handshake failed: %w", err)
	}

	log.Printf("Guacamole handshake complete, starting relay to %s", p.guacdAddr)

	// Bidirectional relay
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	// WebSocket client -> guacd TCP
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := p.relayWSToTCP(clientConn, guacdConn); err != nil {
			errCh <- fmt.Errorf("ws->tcp: %w", err)
		}
	}()

	// guacd TCP -> WebSocket client
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := p.relayTCPToWS(guacdConn, clientConn); err != nil {
			errCh <- fmt.Errorf("tcp->ws: %w", err)
		}
	}()

	// Wait for either direction to finish
	go func() {
		wg.Wait()
		close(errCh)
	}()

	if err := <-errCh; err != nil {
		return err
	}
	return nil
}

// handshake performs the Guacamole protocol handshake with guacd.
// The handshake flow is:
// 1. Send "select" instruction to choose protocol (rdp)
// 2. Read guacd's "args" response listing required parameters
// 3. Send client capability instructions (size, audio, video, image, timezone)
// 4. Send "connect" instruction with RDP parameters
// 5. Read guacd's "ready" response
func (p *GuacdProxy) handshake(conn net.Conn) error {
	// Step 1: Send select instruction
	selectInstr := encodeInstruction("select", "rdp")
	if _, err := conn.Write([]byte(selectInstr)); err != nil {
		return fmt.Errorf("failed to send select: %w", err)
	}

	// Step 2: Read args response
	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to read args: %w", err)
	}
	argsResponse := string(buf[:n])
	log.Printf("guacd args response: %s", argsResponse)

	// Step 3: Send client capability instructions before connect.
	// In the Guacamole protocol, clients must declare their display size and
	// supported media types BEFORE the connect instruction. guacd uses these
	// to set the user's optimal resolution for the RDP session.
	clientInstrs := encodeInstruction("size", p.width, p.height, "96") +
		encodeInstruction("audio") +
		encodeInstruction("video") +
		encodeInstruction("image", "image/png", "image/jpeg", "image/webp") +
		encodeInstruction("timezone", "UTC")
	if _, err := conn.Write([]byte(clientInstrs)); err != nil {
		return fmt.Errorf("failed to send client instructions: %w", err)
	}

	// Step 4: Send connect instruction with RDP parameters
	args := parseInstruction(argsResponse)
	connectArgs := p.buildConnectArgs(args)
	connectInstr := encodeInstruction("connect", connectArgs...)
	if _, err := conn.Write([]byte(connectInstr)); err != nil {
		return fmt.Errorf("failed to send connect: %w", err)
	}

	// Step 5: Read ready response
	n, err = conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to read ready: %w", err)
	}
	readyResponse := string(buf[:n])
	log.Printf("guacd ready response: %s", readyResponse)

	return nil
}

// buildConnectArgs maps guacd's requested parameter names to values.
func (p *GuacdProxy) buildConnectArgs(argNames []string) []string {
	paramMap := map[string]string{
		"VERSION_1_5_0": "VERSION_1_5_0",
		"hostname":      p.hostname,
		"port":          p.port,
		"username":      p.username,
		"password":      p.password,
		"width":         p.width,
		"height":        p.height,
		"dpi":           "96",
		"color-depth":   "24",
		"security":      "rdp",
		"ignore-cert":   "true",
		"disable-auth":  "true",
		"resize-method": "display-update",
	}

	result := make([]string, len(argNames))
	for i, name := range argNames {
		if val, ok := paramMap[name]; ok {
			result[i] = val
		} else {
			result[i] = ""
		}
	}
	return result
}

// relayWSToTCP reads text messages from the WebSocket and writes them to the TCP connection.
func (p *GuacdProxy) relayWSToTCP(ws *websocket.Conn, tcp net.Conn) error {
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) || err == io.EOF {
				return nil
			}
			return err
		}
		if len(message) == 0 {
			continue
		}
		if _, err := tcp.Write(message); err != nil {
			return err
		}
	}
}

// relayTCPToWS reads from the TCP connection and writes text messages to the WebSocket.
func (p *GuacdProxy) relayTCPToWS(tcp net.Conn, ws *websocket.Conn) error {
	buf := make([]byte, 8192)
	for {
		n, err := tcp.Read(buf)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := ws.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
			return err
		}
	}
}

// encodeInstruction encodes a Guacamole protocol instruction.
// Format: opcode_len.opcode,arg1_len.arg1,arg2_len.arg2,...;
func encodeInstruction(opcode string, args ...string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, fmt.Sprintf("%d.%s", len(opcode), opcode))
	for _, arg := range args {
		parts = append(parts, fmt.Sprintf("%d.%s", len(arg), arg))
	}
	return strings.Join(parts, ",") + ";"
}

// parseInstruction extracts the argument names from a guacd "args" instruction.
// The args instruction format: 4.args,N.param1,N.param2,...;
func parseInstruction(raw string) []string {
	var args []string
	i := 0
	first := true // skip the opcode

	for i < len(raw) {
		// Find the length prefix
		dotIdx := -1
		for j := i; j < len(raw); j++ {
			if raw[j] == '.' {
				dotIdx = j
				break
			}
		}
		if dotIdx == -1 {
			break
		}

		// Parse length
		lenStr := raw[i:dotIdx]
		argLen := 0
		for _, c := range lenStr {
			if c >= '0' && c <= '9' {
				argLen = argLen*10 + int(c-'0')
			}
		}

		// Extract value
		valueStart := dotIdx + 1
		valueEnd := valueStart + argLen
		if valueEnd > len(raw) {
			break
		}
		value := raw[valueStart:valueEnd]

		if first {
			first = false // skip the opcode (e.g., "args")
		} else {
			args = append(args, value)
		}

		// Move past the value and the delimiter (',' or ';')
		i = valueEnd + 1
		if i <= len(raw) && (raw[i-1] == ';') {
			break
		}
	}

	return args
}
