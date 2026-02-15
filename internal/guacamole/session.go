package guacamole

import (
	"bytes"
	"io"
	"log"
	"net"
	"sync"

	"github.com/gorilla/websocket"
)

// Client represents a single WebSocket viewer connected to a SharedSession.
type Client struct {
	conn     *websocket.Conn
	viewOnly bool
	writeMu  sync.Mutex   // serializes WS writes (broadcast + close frames)
	done     chan struct{} // closed when client disconnects
	closeOnce sync.Once
}

// close signals that this client has disconnected.
func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
	})
}

// writeMessage sends a WebSocket message to the client, serialized by writeMu.
func (c *Client) writeMessage(msgType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(msgType, data)
}

// maxDisplayBuf caps the display replay buffer at 16 MB. When the buffer
// exceeds this, old data is discarded. Late-joining clients may see a
// partially rendered screen until the next full redraw.
const maxDisplayBuf = 16 * 1024 * 1024

// SharedSession maintains exactly one guacd TCP connection and broadcasts
// display data to all connected WebSocket clients. Input from non-read-only
// clients is multiplexed onto the single guacd connection.
type SharedSession struct {
	sessionID string
	guacdConn net.Conn
	excess    []byte // initial display data from handshake

	mu      sync.RWMutex
	clients map[*Client]struct{}

	// displayBuf accumulates all display data sent by guacd so that
	// late-joining clients can replay it and see the current screen.
	// Protected by mu (writes use full Lock, reads use RLock).
	displayBuf []byte

	inputMu sync.Mutex // serializes writes to guacdConn from multiple clients

	closeOnce sync.Once
	done      chan struct{} // closed when session is torn down

	onClose func() // callback to remove from registry
}

// newSharedSession dials guacd, performs the handshake, and starts the
// broadcast loop. The onClose callback is invoked once when the session closes.
func newSharedSession(sessionID, guacdAddr, hostname, port, username, password, width, height string, onClose func()) (*SharedSession, error) {
	guacdConn, err := net.Dial("tcp", guacdAddr)
	if err != nil {
		return nil, err
	}

	excess, err := performHandshake(guacdConn, hostname, port, username, password, width, height)
	if err != nil {
		guacdConn.Close()
		return nil, err
	}

	log.Printf("SharedSession %s: guacd handshake complete at %s", sessionID, guacdAddr)

	// Seed the display buffer with any excess data from the handshake
	var displayBuf []byte
	if len(excess) > 0 {
		displayBuf = make([]byte, len(excess))
		copy(displayBuf, excess)
	}

	s := &SharedSession{
		sessionID:  sessionID,
		guacdConn:  guacdConn,
		excess:     excess,
		displayBuf: displayBuf,
		clients:    make(map[*Client]struct{}),
		done:       make(chan struct{}),
		onClose:    onClose,
	}

	go s.broadcastLoop()

	return s, nil
}

// AddClient registers a new WebSocket connection and blocks until the client
// disconnects. The caller's goroutine is consumed for the lifetime of the client.
func (s *SharedSession) AddClient(conn *websocket.Conn, viewOnly bool) {
	client := &Client{
		conn:     conn,
		viewOnly: viewOnly,
		done:     make(chan struct{}),
	}

	// Hold the write lock while replaying the display buffer AND adding
	// the client. This ensures no broadcast data is lost between the
	// replay and joining the live broadcast list.
	s.mu.Lock()
	select {
	case <-s.done:
		s.mu.Unlock()
		return
	default:
	}

	// Replay accumulated display data so the new client sees the current screen.
	// This includes the handshake excess plus everything broadcastLoop has sent.
	replayData := s.displayBuf
	if len(replayData) > 0 {
		if err := client.writeMessage(websocket.TextMessage, replayData); err != nil {
			s.mu.Unlock()
			log.Printf("SharedSession %s: failed to replay display to client: %v", s.sessionID, err)
			return
		}
	}

	s.clients[client] = struct{}{}
	s.mu.Unlock()

	log.Printf("SharedSession %s: client added (viewOnly=%v, total=%d)", s.sessionID, viewOnly, s.clientCount())

	go s.handleClientInput(client)

	// Block until client disconnects
	<-client.done

	s.RemoveClient(client)
}

// RemoveClient removes a client. If it was the last client, the session closes.
func (s *SharedSession) RemoveClient(client *Client) {
	s.mu.Lock()
	_, exists := s.clients[client]
	if exists {
		delete(s.clients, client)
	}
	remaining := len(s.clients)
	s.mu.Unlock()

	if !exists {
		return
	}

	log.Printf("SharedSession %s: client removed (remaining=%d)", s.sessionID, remaining)

	if remaining == 0 {
		s.Close()
	}
}

// Close tears down the shared session: closes the guacd connection and all clients.
func (s *SharedSession) Close() {
	s.closeOnce.Do(func() {
		log.Printf("SharedSession %s: closing", s.sessionID)
		close(s.done)
		s.guacdConn.Close()

		s.mu.RLock()
		clients := make([]*Client, 0, len(s.clients))
		for c := range s.clients {
			clients = append(clients, c)
		}
		s.mu.RUnlock()

		for _, c := range clients {
			c.conn.Close()
			c.close()
		}

		if s.onClose != nil {
			s.onClose()
		}
	})
}

// broadcastLoop reads complete Guacamole instructions from the guacd TCP
// connection and writes them to all connected clients. This is the same
// instruction-buffering logic as relayTCPToWS but broadcasting to N clients.
func (s *SharedSession) broadcastLoop() {
	buf := make([]byte, 65536)
	var carry []byte

	for {
		n, err := s.guacdConn.Read(buf)
		if err != nil {
			if len(carry) > 0 {
				s.broadcast(carry)
			}
			if err != io.EOF {
				log.Printf("SharedSession %s: guacd read error: %v", s.sessionID, err)
			}
			s.Close()
			return
		}

		var data []byte
		if len(carry) > 0 {
			data = make([]byte, len(carry)+n)
			copy(data, carry)
			copy(data[len(carry):], buf[:n])
			carry = nil
		} else {
			data = buf[:n]
		}

		lastSemi := bytes.LastIndexByte(data, ';')
		if lastSemi < 0 {
			carry = make([]byte, len(data))
			copy(carry, data)
			continue
		}

		toSend := data[:lastSemi+1]
		// Make a copy since buf will be reused
		msg := make([]byte, len(toSend))
		copy(msg, toSend)
		s.broadcast(msg)

		if lastSemi+1 < len(data) {
			remaining := data[lastSemi+1:]
			carry = make([]byte, len(remaining))
			copy(carry, remaining)
		}
	}
}

// broadcast appends data to the display buffer and sends it to all connected
// clients, removing any that error.
func (s *SharedSession) broadcast(data []byte) {
	s.mu.Lock()
	s.displayBuf = append(s.displayBuf, data...)
	// Cap the buffer to prevent unbounded growth. Discard the oldest half
	// when the limit is exceeded so late joiners still get a useful replay.
	if len(s.displayBuf) > maxDisplayBuf {
		half := len(s.displayBuf) / 2
		copy(s.displayBuf, s.displayBuf[half:])
		s.displayBuf = s.displayBuf[:len(s.displayBuf)-half]
	}
	clients := make([]*Client, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()

	for _, c := range clients {
		if err := c.writeMessage(websocket.TextMessage, data); err != nil {
			log.Printf("SharedSession %s: broadcast write error, removing client: %v", s.sessionID, err)
			c.conn.Close()
			c.close()
		}
	}
}

// handleClientInput reads from the client's WebSocket. For non-view-only
// clients, input is forwarded to guacd. For view-only clients, input is
// discarded. This goroutine also detects client disconnection.
func (s *SharedSession) handleClientInput(client *Client) {
	defer client.close()

	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) && err != io.EOF {
				log.Printf("SharedSession %s: client read error: %v", s.sessionID, err)
			}
			return
		}

		if client.viewOnly || len(message) == 0 {
			continue
		}

		s.inputMu.Lock()
		_, err = s.guacdConn.Write(message)
		s.inputMu.Unlock()
		if err != nil {
			log.Printf("SharedSession %s: guacd write error: %v", s.sessionID, err)
			return
		}
	}
}

func (s *SharedSession) clientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// SessionRegistry is a thread-safe map of session ID → SharedSession.
// It ensures exactly one guacd connection exists per Sortie session.
type SessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]*SharedSession
}

// NewSessionRegistry creates a new empty registry.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		sessions: make(map[string]*SharedSession),
	}
}

// GetOrCreate returns the existing SharedSession for the given session ID,
// or creates a new one by dialing guacd and performing the handshake.
func (r *SessionRegistry) GetOrCreate(sessionID, guacdAddr, hostname, port, username, password, width, height string) (*SharedSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[sessionID]; ok {
		// Verify session is still alive
		select {
		case <-s.done:
			// Session closed, fall through to create new one
			delete(r.sessions, sessionID)
		default:
			return s, nil
		}
	}

	s, err := newSharedSession(sessionID, guacdAddr, hostname, port, username, password, width, height, func() {
		r.mu.Lock()
		delete(r.sessions, sessionID)
		r.mu.Unlock()
	})
	if err != nil {
		return nil, err
	}

	r.sessions[sessionID] = s
	return s, nil
}
