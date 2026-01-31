package websocket

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// In production, this should be restricted
		return true
	},
}

// Proxy handles bidirectional WebSocket proxying
type Proxy struct {
	targetURL string
}

// NewProxy creates a new WebSocket proxy
func NewProxy(targetURL string) *Proxy {
	return &Proxy{
		targetURL: targetURL,
	}
}

// ServeHTTP upgrades the connection and starts proxying
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Upgrade the client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade client connection: %v", err)
		return
	}
	defer clientConn.Close()

	// Parse the target URL
	targetURL, err := url.Parse(p.targetURL)
	if err != nil {
		log.Printf("Invalid target URL: %v", err)
		return
	}

	// Connect to the target WebSocket server
	dialer := websocket.Dialer{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}

	targetConn, _, err := dialer.Dial(targetURL.String(), nil)
	if err != nil {
		log.Printf("Failed to connect to target %s: %v", p.targetURL, err)
		clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to VNC server"))
		return
	}
	defer targetConn.Close()

	// Start bidirectional proxying
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	// Client -> Target
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := proxyMessages(clientConn, targetConn); err != nil {
			errCh <- err
		}
	}()

	// Target -> Client
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := proxyMessages(targetConn, clientConn); err != nil {
			errCh <- err
		}
	}()

	// Wait for either direction to finish
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Log first error
	if err := <-errCh; err != nil && !isCloseError(err) {
		log.Printf("WebSocket proxy error: %v", err)
	}
}

// proxyMessages copies messages from src to dst
func proxyMessages(src, dst *websocket.Conn) error {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			return err
		}

		if err := dst.WriteMessage(messageType, message); err != nil {
			return err
		}
	}
}

// isCloseError checks if the error is a normal close error
func isCloseError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return true
	}
	return false
}
