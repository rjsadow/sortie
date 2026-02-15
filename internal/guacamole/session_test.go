package guacamole

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeGuacd simulates a guacd server for testing. It performs a minimal
// Guacamole handshake and then allows tests to send/receive data.
type fakeGuacd struct {
	listener net.Listener
	conn     net.Conn // set after accept
	mu       sync.Mutex
}

func newFakeGuacd(t *testing.T) *fakeGuacd {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake guacd: %v", err)
	}
	f := &fakeGuacd{listener: l}
	t.Cleanup(func() {
		f.closeConn()
		l.Close()
	})
	return f
}

func (f *fakeGuacd) addr() string {
	return f.listener.Addr().String()
}

// acceptAndHandshake accepts one connection and performs the guacd side of the
// Guacamole handshake: sends args, reads client instructions, sends ready.
func (f *fakeGuacd) acceptAndHandshake(t *testing.T) {
	t.Helper()
	conn, err := f.listener.Accept()
	if err != nil {
		t.Fatalf("fake guacd accept failed: %v", err)
	}
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()

	// Read "select" instruction
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("fake guacd: failed to read select: %v", err)
	}
	selectInstr := string(buf[:n])
	if !strings.Contains(selectInstr, "select") {
		t.Fatalf("fake guacd: expected select, got: %s", selectInstr)
	}

	// Send args response
	argsInstr := encodeInstruction("args", "hostname", "port", "username", "password", "width", "height")
	if _, err := conn.Write([]byte(argsInstr)); err != nil {
		t.Fatalf("fake guacd: failed to send args: %v", err)
	}

	// Read client capability instructions + connect.
	// The client sends these in two separate Write calls, so we may need
	// two reads to consume both the capability instructions and the connect.
	var handshakeData string
	for !strings.Contains(handshakeData, "connect") {
		n, err = conn.Read(buf)
		if err != nil {
			t.Fatalf("fake guacd: failed to read client instrs: %v", err)
		}
		handshakeData += string(buf[:n])
	}

	// Send ready response
	readyInstr := encodeInstruction("ready", "test-conn-id")
	if _, err := conn.Write([]byte(readyInstr)); err != nil {
		t.Fatalf("fake guacd: failed to send ready: %v", err)
	}
}

// send writes data to the accepted connection (simulates guacd display output).
func (f *fakeGuacd) send(t *testing.T, data string) {
	t.Helper()
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	if _, err := conn.Write([]byte(data)); err != nil {
		t.Fatalf("fake guacd: send failed: %v", err)
	}
}

// read reads data from the accepted connection (simulates guacd receiving input).
func (f *fakeGuacd) read(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("fake guacd: read failed: %v", err)
	}
	return string(buf[:n])
}

// closeConn closes the fake guacd's connection to simulate guacd disconnect.
func (f *fakeGuacd) closeConn() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn != nil {
		f.conn.Close()
	}
}

// wsDialer creates a WebSocket client connected to the given server URL.
func wsDialer(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("ws dial failed: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("ws dial: unexpected status %d", resp.StatusCode)
	}
	return conn
}

// waitForClients polls until the shared session has the expected number of clients.
func waitForClients(t *testing.T, s *SharedSession, expected int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.clientCount() == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d clients, have %d", expected, s.clientCount())
}

// createWSPair creates a connected pair of WebSocket connections using an in-process server.
func createWSPair(t *testing.T) (client *websocket.Conn, server *websocket.Conn) {
	t.Helper()

	done := make(chan struct{})
	serverReady := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverReady <- c
		<-done
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn := wsDialer(t, wsURL)

	// Cleanup in LIFO order: close client, unblock handler, close server
	t.Cleanup(func() {
		clientConn.Close()
		close(done)
		srv.Close()
	})

	select {
	case serverConn := <-serverReady:
		return clientConn, serverConn
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server-side WS connection")
		return nil, nil
	}
}

func TestSessionRegistry_GetOrCreate(t *testing.T) {
	guacd := newFakeGuacd(t)
	registry := NewSessionRegistry()

	// Start handshake in background
	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	s1, err := registry.GetOrCreate("sess-1", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	// Same ID should return the same session without another handshake
	s2, err := registry.GetOrCreate("sess-1", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate second call failed: %v", err)
	}
	if s1 != s2 {
		t.Error("GetOrCreate returned different sessions for the same ID")
	}

	// Clean up
	s1.Close()
}

func TestSessionRegistry_ClosedSessionRecreated(t *testing.T) {
	guacd := newFakeGuacd(t)
	registry := NewSessionRegistry()

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	s1, err := registry.GetOrCreate("sess-2", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	// Close the session
	s1.Close()

	// Create a new fake guacd for the next connection
	guacd2 := newFakeGuacd(t)
	done2 := make(chan struct{})
	go func() {
		guacd2.acceptAndHandshake(t)
		close(done2)
	}()

	s2, err := registry.GetOrCreate("sess-2", guacd2.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate after close failed: %v", err)
	}
	<-done2

	if s1 == s2 {
		t.Error("GetOrCreate should return a new session after the old one closed")
	}

	s2.Close()
}

func TestSharedSession_MultipleClients(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-multi", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	// Create two WS client pairs
	client1, server1 := createWSPair(t)
	client2, server2 := createWSPair(t)

	// Add both clients in background goroutines (AddClient blocks)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		shared.AddClient(server1, false)
	}()
	go func() {
		defer wg.Done()
		shared.AddClient(server2, false)
	}()

	waitForClients(t, shared, 2)

	// Send display data from guacd — both clients should receive it
	displayData := encodeInstruction("png", "1", "0", "0", "0")
	guacd.send(t, displayData)

	// Read from both WS clients
	for i, c := range []*websocket.Conn{client1, client2} {
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("client %d: read failed: %v", i+1, err)
		}
		if string(msg) != displayData {
			t.Errorf("client %d: got %q, want %q", i+1, string(msg), displayData)
		}
	}

	// Clean up
	client1.Close()
	client2.Close()
	wg.Wait()
}

func TestSharedSession_ViewOnlyInput(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-vo", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	// Create a view-only client and a normal client
	voClient, voServer := createWSPair(t)
	normalClient, normalServer := createWSPair(t)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		shared.AddClient(voServer, true) // view only
	}()
	go func() {
		defer wg.Done()
		shared.AddClient(normalServer, false) // normal
	}()

	waitForClients(t, shared, 2)

	// Normal client sends input — should reach guacd
	inputInstr := encodeInstruction("mouse", "100", "200", "1")
	if err := normalClient.WriteMessage(websocket.TextMessage, []byte(inputInstr)); err != nil {
		t.Fatalf("normal client write failed: %v", err)
	}

	received := guacd.read(t)
	if received != inputInstr {
		t.Errorf("guacd received %q, want %q", received, inputInstr)
	}

	// View-only client sends input — should NOT reach guacd
	voInput := encodeInstruction("key", "65", "1")
	if err := voClient.WriteMessage(websocket.TextMessage, []byte(voInput)); err != nil {
		t.Fatalf("view-only client write failed: %v", err)
	}

	// Give some time for the data to travel (if it were going to)
	time.Sleep(100 * time.Millisecond)

	// Send another normal input to verify guacd doesn't get the view-only input
	inputInstr2 := encodeInstruction("mouse", "300", "400", "0")
	if err := normalClient.WriteMessage(websocket.TextMessage, []byte(inputInstr2)); err != nil {
		t.Fatalf("normal client write 2 failed: %v", err)
	}

	received2 := guacd.read(t)
	if received2 != inputInstr2 {
		t.Errorf("guacd received %q after view-only input, want %q (view-only input leaked)", received2, inputInstr2)
	}

	// Clean up
	voClient.Close()
	normalClient.Close()
	wg.Wait()
}

func TestSharedSession_LastClientCleanup(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-cleanup", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	_, server1 := createWSPair(t)
	client2, server2 := createWSPair(t)

	clientDone := make(chan struct{}, 2)
	go func() {
		shared.AddClient(server1, false)
		clientDone <- struct{}{}
	}()
	go func() {
		shared.AddClient(server2, false)
		clientDone <- struct{}{}
	}()

	waitForClients(t, shared, 2)

	// Close server1's underlying WS — simulates first client disconnect.
	// We close server1 (the server-side connection passed to AddClient).
	server1.Close()
	<-clientDone // wait for AddClient to return

	waitForClients(t, shared, 1)

	// Session should still be alive
	select {
	case <-shared.done:
		t.Fatal("session closed prematurely after first client disconnect")
	default:
	}

	// Close last client
	client2.Close()
	server2.Close()
	<-clientDone

	// Session should now be closed
	select {
	case <-shared.done:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("session did not close after last client disconnect")
	}

	// Registry should be cleaned up
	registry.mu.Lock()
	_, exists := registry.sessions["sess-cleanup"]
	registry.mu.Unlock()
	if exists {
		t.Error("session still in registry after close")
	}
}

func TestSharedSession_GuacdDisconnect(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-guacd-dc", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	client1, server1 := createWSPair(t)
	client2, server2 := createWSPair(t)

	clientDone := make(chan struct{}, 2)
	go func() {
		shared.AddClient(server1, false)
		clientDone <- struct{}{}
	}()
	go func() {
		shared.AddClient(server2, false)
		clientDone <- struct{}{}
	}()

	waitForClients(t, shared, 2)

	// Simulate guacd crashing — close the TCP connection
	guacd.closeConn()

	// Both clients should be disconnected
	<-clientDone
	<-clientDone

	// Session should be closed
	select {
	case <-shared.done:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("session did not close after guacd disconnect")
	}

	// Both WS clients should get close/error on next read
	for i, c := range []*websocket.Conn{client1, client2} {
		c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, _, err := c.ReadMessage()
		if err == nil {
			t.Errorf("client %d: expected error after guacd disconnect, got none", i+1)
		}
	}
}

func TestSharedSession_InstructionBuffering(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-buf", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	client1, server1 := createWSPair(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		shared.AddClient(server1, false)
	}()

	waitForClients(t, shared, 1)

	// Send a partial instruction followed by the rest
	instr1 := encodeInstruction("png", "1", "0", "0", "0")
	instr2 := encodeInstruction("sync", "12345")
	combined := instr1 + instr2

	// Split the combined data in the middle of the second instruction
	splitPoint := len(instr1) + 3 // partial into "sync"
	part1 := combined[:splitPoint]
	part2 := combined[splitPoint:]

	// Send first part (complete instr1 + partial instr2)
	guacd.send(t, part1)

	// Read should get only the complete instruction
	client1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := client1.ReadMessage()
	if err != nil {
		t.Fatalf("client read 1 failed: %v", err)
	}
	if string(msg1) != instr1 {
		t.Errorf("first read: got %q, want %q", string(msg1), instr1)
	}

	// Send the rest
	guacd.send(t, part2)

	// Read should get the completed second instruction
	client1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg2, err := client1.ReadMessage()
	if err != nil {
		t.Fatalf("client read 2 failed: %v", err)
	}
	if string(msg2) != instr2 {
		t.Errorf("second read: got %q, want %q", string(msg2), instr2)
	}

	// Clean up
	client1.Close()
	wg.Wait()
}

func TestSharedSession_ExcessDataSentToClient(t *testing.T) {
	// Test that initial display data from the handshake (excess) is sent
	// to clients when they connect.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	excessData := encodeInstruction("png", "1", "0", "0", "0")

	done := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		// Read select
		buf := make([]byte, 4096)
		conn.Read(buf)

		// Send args
		argsInstr := encodeInstruction("args", "hostname", "port")
		conn.Write([]byte(argsInstr))

		// Read client instrs + connect
		conn.Read(buf)

		// Send ready + excess display data in one write
		readyInstr := encodeInstruction("ready", "conn-1")
		conn.Write([]byte(readyInstr + excessData))
		close(done)

		// Keep connection alive for broadcast loop
		io.ReadAll(conn)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-excess", listener.Addr().String(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done
	defer shared.Close()

	// Connect a client
	client, server := createWSPair(t)
	defer client.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		shared.AddClient(server, false)
	}()

	// Client should receive the excess data as the first message
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("client read excess failed: %v", err)
	}
	if string(msg) != excessData {
		t.Errorf("excess data: got %q, want %q", string(msg), excessData)
	}

	client.Close()
	wg.Wait()
}

func TestSharedSession_LateJoinerSeesDisplay(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-late", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	// First client connects
	client1, server1 := createWSPair(t)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		shared.AddClient(server1, false)
	}()

	waitForClients(t, shared, 1)

	// guacd sends display data — only client1 is connected
	instr1 := encodeInstruction("png", "1", "0", "0", "0")
	instr2 := encodeInstruction("cfill", "0", "0", "1920", "1080", "0", "0", "0", "255")
	instr3 := encodeInstruction("sync", "12345")
	guacd.send(t, instr1+instr2+instr3)

	// Client1 receives the display data
	client1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := client1.ReadMessage()
	if err != nil {
		t.Fatalf("client1 read failed: %v", err)
	}
	expectedAll := instr1 + instr2 + instr3
	if string(msg1) != expectedAll {
		t.Fatalf("client1 got %q, want %q", string(msg1), expectedAll)
	}

	// Second client connects AFTER the display was rendered
	client2, server2 := createWSPair(t)
	wg.Add(1)
	go func() {
		defer wg.Done()
		shared.AddClient(server2, false)
	}()

	waitForClients(t, shared, 2)

	// Client2 should receive a replay of all display data as its first message
	client2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg2, err := client2.ReadMessage()
	if err != nil {
		t.Fatalf("client2 replay read failed: %v", err)
	}
	if string(msg2) != expectedAll {
		t.Errorf("client2 replay: got %q, want %q", string(msg2), expectedAll)
	}

	// New live data should also reach client2
	liveInstr := encodeInstruction("sync", "99999")
	guacd.send(t, liveInstr)

	for i, c := range []*websocket.Conn{client1, client2} {
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("client %d live read failed: %v", i+1, err)
		}
		if string(msg) != liveInstr {
			t.Errorf("client %d live: got %q, want %q", i+1, string(msg), liveInstr)
		}
	}

	// Clean up
	client1.Close()
	client2.Close()
	wg.Wait()
}

func TestSharedSession_DisplayBufferCap(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-cap", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done
	defer shared.Close()

	// Directly fill the display buffer beyond maxDisplayBuf to test truncation.
	// We don't need a connected client — broadcast() writes to displayBuf regardless.
	bigChunk := make([]byte, maxDisplayBuf/2+1)
	for i := range bigChunk {
		bigChunk[i] = 'A'
	}
	// Terminate as a valid instruction so broadcast sends it
	bigChunk[len(bigChunk)-1] = ';'

	// First broadcast — buffer should be under the cap
	shared.broadcast(bigChunk)
	shared.mu.RLock()
	size1 := len(shared.displayBuf)
	shared.mu.RUnlock()
	if size1 != len(bigChunk) {
		t.Fatalf("after first broadcast: displayBuf size = %d, want %d", size1, len(bigChunk))
	}

	// Second broadcast — total would exceed maxDisplayBuf, should trigger truncation
	shared.broadcast(bigChunk)
	shared.mu.RLock()
	size2 := len(shared.displayBuf)
	shared.mu.RUnlock()

	if size2 > maxDisplayBuf {
		t.Errorf("displayBuf size %d exceeds maxDisplayBuf %d after truncation", size2, maxDisplayBuf)
	}
	if size2 == 0 {
		t.Error("displayBuf should not be empty after truncation")
	}
}

func TestSharedSession_AddClientAfterClose(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-closed", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done

	// Close the session first
	shared.Close()

	// Now try to add a client — should return immediately without blocking
	_, server := createWSPair(t)
	addDone := make(chan struct{})
	go func() {
		shared.AddClient(server, false)
		close(addDone)
	}()

	select {
	case <-addDone:
		// AddClient returned immediately — correct behavior
	case <-time.After(2 * time.Second):
		t.Fatal("AddClient blocked on a closed session instead of returning immediately")
	}

	// No clients should have been added
	if shared.clientCount() != 0 {
		t.Errorf("clientCount = %d, want 0 for closed session", shared.clientCount())
	}
}

func TestSharedSession_ConcurrentInputSerialization(t *testing.T) {
	guacd := newFakeGuacd(t)

	done := make(chan struct{})
	go func() {
		guacd.acceptAndHandshake(t)
		close(done)
	}()

	registry := NewSessionRegistry()
	shared, err := registry.GetOrCreate("sess-input", guacd.addr(), "127.0.0.1", "3389", "u", "p", "1024", "768")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}
	<-done
	defer shared.Close()

	// Create multiple non-view-only clients
	const numClients = 3
	clients := make([]*websocket.Conn, numClients)
	servers := make([]*websocket.Conn, numClients)
	var wg sync.WaitGroup

	for i := range numClients {
		c, s := createWSPair(t)
		clients[i] = c
		servers[i] = s
		wg.Add(1)
		go func(s *websocket.Conn) {
			defer wg.Done()
			shared.AddClient(s, false)
		}(s)
	}

	waitForClients(t, shared, numClients)

	// All clients send input concurrently
	const messagesPerClient = 10
	var sendWg sync.WaitGroup
	for i := range numClients {
		sendWg.Add(1)
		go func(clientIdx int, c *websocket.Conn) {
			defer sendWg.Done()
			for j := range messagesPerClient {
				instr := encodeInstruction("mouse", fmt.Sprintf("%d", clientIdx*100+j), "0", "0")
				c.WriteMessage(websocket.TextMessage, []byte(instr))
			}
		}(i, clients[i])
	}
	sendWg.Wait()

	// Read all messages from guacd — each should be a complete instruction
	totalExpected := numClients * messagesPerClient
	received := 0
	guacd.mu.Lock()
	conn := guacd.conn
	guacd.mu.Unlock()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	var allData []byte
	buf := make([]byte, 65536)
	for received < totalExpected {
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		allData = append(allData, buf[:n]...)
		// Count complete instructions
		received = strings.Count(string(allData), ";")
	}

	if received != totalExpected {
		t.Errorf("guacd received %d instructions, want %d", received, totalExpected)
	}

	// Clean up
	for _, c := range clients {
		c.Close()
	}
	wg.Wait()
}
