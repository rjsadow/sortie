package websocket

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewProxy(t *testing.T) {
	p := NewProxy("ws://example.com:6080")
	if p == nil {
		t.Fatal("NewProxy() returned nil")
	}
	if p.targetURL != "ws://example.com:6080" {
		t.Errorf("got targetURL = %q, want %q", p.targetURL, "ws://example.com:6080")
	}
}

func TestIsCloseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "normal close",
			err:  &websocket.CloseError{Code: websocket.CloseNormalClosure},
			want: true,
		},
		{
			name: "going away",
			err:  &websocket.CloseError{Code: websocket.CloseGoingAway},
			want: true,
		},
		{
			name: "abnormal close",
			err:  &websocket.CloseError{Code: websocket.CloseAbnormalClosure},
			want: false,
		},
		{
			name: "internal server error",
			err:  &websocket.CloseError{Code: websocket.CloseInternalServerErr},
			want: false,
		},
		{
			name: "generic error",
			err:  io.ErrUnexpectedEOF,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCloseError(tt.err); got != tt.want {
				t.Errorf("isCloseError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// echoServer creates a WebSocket server that echoes messages back.
func echoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("echo server upgrade error: %v", err)
			return
		}
		defer conn.Close()
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(messageType, message); err != nil {
				return
			}
		}
	}))
}

func TestProxy_BidirectionalMessages(t *testing.T) {
	// Start an echo WebSocket server as the "target" (simulates VNC backend)
	echoSrv := echoServer(t)
	defer echoSrv.Close()

	targetURL := "ws" + strings.TrimPrefix(echoSrv.URL, "http")

	// Create the proxy and wrap it in a test server
	proxy := NewProxy(targetURL)
	proxySrv := httptest.NewServer(http.HandlerFunc(proxy.ServeHTTP))
	defer proxySrv.Close()

	// Connect a client to the proxy
	proxyURL := "ws" + strings.TrimPrefix(proxySrv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer clientConn.Close()

	// Send text messages and verify echo
	testMessages := []string{"hello", "world", "test message with spaces"}
	for _, msg := range testMessages {
		if err := clientConn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			t.Fatalf("failed to write message: %v", err)
		}

		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		messageType, received, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read message: %v", err)
		}

		if messageType != websocket.TextMessage {
			t.Errorf("got message type %d, want %d", messageType, websocket.TextMessage)
		}
		if string(received) != msg {
			t.Errorf("got message %q, want %q", string(received), msg)
		}
	}
}

func TestProxy_BinaryMessages(t *testing.T) {
	echoSrv := echoServer(t)
	defer echoSrv.Close()

	targetURL := "ws" + strings.TrimPrefix(echoSrv.URL, "http")
	proxy := NewProxy(targetURL)
	proxySrv := httptest.NewServer(http.HandlerFunc(proxy.ServeHTTP))
	defer proxySrv.Close()

	proxyURL := "ws" + strings.TrimPrefix(proxySrv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer clientConn.Close()

	// Send binary message (simulates VNC data)
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
	if err := clientConn.WriteMessage(websocket.BinaryMessage, binaryData); err != nil {
		t.Fatalf("failed to write binary message: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	messageType, received, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read binary message: %v", err)
	}

	if messageType != websocket.BinaryMessage {
		t.Errorf("got message type %d, want %d", messageType, websocket.BinaryMessage)
	}
	if len(received) != len(binaryData) {
		t.Fatalf("got %d bytes, want %d", len(received), len(binaryData))
	}
	for i, b := range received {
		if b != binaryData[i] {
			t.Errorf("byte[%d] = %02x, want %02x", i, b, binaryData[i])
		}
	}
}

func TestProxy_MultipleMessages(t *testing.T) {
	echoSrv := echoServer(t)
	defer echoSrv.Close()

	targetURL := "ws" + strings.TrimPrefix(echoSrv.URL, "http")
	proxy := NewProxy(targetURL)
	proxySrv := httptest.NewServer(http.HandlerFunc(proxy.ServeHTTP))
	defer proxySrv.Close()

	proxyURL := "ws" + strings.TrimPrefix(proxySrv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer clientConn.Close()

	// Send many messages rapidly
	messageCount := 50
	for i := range messageCount {
		msg := []byte{byte(i)}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}

		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, received, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read message %d: %v", i, err)
		}
		if len(received) != 1 || received[0] != byte(i) {
			t.Errorf("message %d: got %v, want [%d]", i, received, i)
		}
	}
}

func TestProxy_ClientClose(t *testing.T) {
	echoSrv := echoServer(t)
	defer echoSrv.Close()

	targetURL := "ws" + strings.TrimPrefix(echoSrv.URL, "http")
	proxy := NewProxy(targetURL)
	proxySrv := httptest.NewServer(http.HandlerFunc(proxy.ServeHTTP))
	defer proxySrv.Close()

	proxyURL := "ws" + strings.TrimPrefix(proxySrv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}

	// Send a message to make sure connection is working
	if err := clientConn.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read echo: %v", err)
	}

	// Close the client connection gracefully
	err = clientConn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	if err != nil {
		t.Fatalf("failed to send close message: %v", err)
	}
}

func TestProxy_InvalidTargetURL(t *testing.T) {
	// Proxy with an unreachable target - the proxy should handle this gracefully
	proxy := NewProxy("ws://127.0.0.1:1") // port 1 is typically not listening
	proxySrv := httptest.NewServer(http.HandlerFunc(proxy.ServeHTTP))
	defer proxySrv.Close()

	proxyURL := "ws" + strings.TrimPrefix(proxySrv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer clientConn.Close()

	// The proxy should close the connection after failing to connect to target
	clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = clientConn.ReadMessage()
	if err == nil {
		t.Error("expected error reading from proxy with bad target, got nil")
	}
}

func TestProxy_EmptyMessage(t *testing.T) {
	echoSrv := echoServer(t)
	defer echoSrv.Close()

	targetURL := "ws" + strings.TrimPrefix(echoSrv.URL, "http")
	proxy := NewProxy(targetURL)
	proxySrv := httptest.NewServer(http.HandlerFunc(proxy.ServeHTTP))
	defer proxySrv.Close()

	proxyURL := "ws" + strings.TrimPrefix(proxySrv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer clientConn.Close()

	// Send empty message
	if err := clientConn.WriteMessage(websocket.TextMessage, []byte{}); err != nil {
		t.Fatalf("failed to write empty message: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, received, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read empty message echo: %v", err)
	}
	if len(received) != 0 {
		t.Errorf("got %d bytes, want 0", len(received))
	}
}

func TestProxy_LargeMessage(t *testing.T) {
	echoSrv := echoServer(t)
	defer echoSrv.Close()

	targetURL := "ws" + strings.TrimPrefix(echoSrv.URL, "http")
	proxy := NewProxy(targetURL)
	proxySrv := httptest.NewServer(http.HandlerFunc(proxy.ServeHTTP))
	defer proxySrv.Close()

	proxyURL := "ws" + strings.TrimPrefix(proxySrv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(proxyURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer clientConn.Close()

	// Send a large binary message (simulate a VNC frame)
	largeData := make([]byte, 64*1024) // 64KB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	if err := clientConn.WriteMessage(websocket.BinaryMessage, largeData); err != nil {
		t.Fatalf("failed to write large message: %v", err)
	}

	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, received, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read large message echo: %v", err)
	}
	if len(received) != len(largeData) {
		t.Fatalf("got %d bytes, want %d", len(received), len(largeData))
	}
}

func TestUpgraderConfig(t *testing.T) {
	// Verify the package-level upgrader configuration
	if upgrader.ReadBufferSize != 4096 {
		t.Errorf("ReadBufferSize = %d, want 4096", upgrader.ReadBufferSize)
	}
	if upgrader.WriteBufferSize != 4096 {
		t.Errorf("WriteBufferSize = %d, want 4096", upgrader.WriteBufferSize)
	}
	if len(upgrader.Subprotocols) != 1 || upgrader.Subprotocols[0] != "binary" {
		t.Errorf("Subprotocols = %v, want [binary]", upgrader.Subprotocols)
	}
	// CheckOrigin should allow all origins (returns true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !upgrader.CheckOrigin(req) {
		t.Error("CheckOrigin() returned false, want true")
	}
}
