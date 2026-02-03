package proxy

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/sessions"
)

// HTTPProxy handles reverse proxying HTTP requests to web_proxy session pods
type HTTPProxy struct {
	sessionManager *sessions.Manager
}

// NewHTTPProxy creates a new HTTP proxy handler
func NewHTTPProxy(sm *sessions.Manager) *HTTPProxy {
	return &HTTPProxy{
		sessionManager: sm,
	}
}

// ServeHTTP handles HTTP proxy requests
// Expected path format: /api/sessions/{id}/proxy/...
func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path
	sessionID := extractSessionID(r.URL.Path)
	if sessionID == "" {
		http.Error(w, "Invalid session path", http.StatusBadRequest)
		return
	}

	// Get session
	session, err := p.sessionManager.GetSession(r.Context(), sessionID)
	if err != nil {
		log.Printf("Error getting session %s: %v", sessionID, err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Check session status
	if session.Status != db.SessionStatusRunning {
		http.Error(w, "Session not available", http.StatusServiceUnavailable)
		return
	}

	// Get the target URL
	targetURL := p.sessionManager.GetPodProxyEndpoint(session)
	if targetURL == "" {
		http.Error(w, "Session not ready", http.StatusServiceUnavailable)
		return
	}

	// Parse target URL
	target, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("Error parsing target URL %s: %v", targetURL, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Strip the proxy prefix from the path
	proxyPath := stripProxyPrefix(r.URL.Path, sessionID)

	// Check if this is a WebSocket upgrade request
	if isWebSocketRequest(r) {
		p.handleWebSocket(w, r, target, proxyPath, sessionID)
		return
	}

	// Create reverse proxy for HTTP requests
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Configure transport for internal pod connections (skip TLS verification for self-signed certs)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Internal pod connections use self-signed certs
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Customize the director to rewrite the request
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = proxyPath
		req.URL.RawPath = proxyPath
		req.Host = target.Host

		// Preserve the original query string
		req.URL.RawQuery = r.URL.RawQuery

		// Remove the proxy authorization header if present (we already authenticated)
		req.Header.Del("Authorization")

		// Set headers to help the backend understand the proxy context
		req.Header.Set("X-Forwarded-Host", r.Host)
		req.Header.Set("X-Forwarded-Proto", "http")
		if r.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		}
		// Add X-Real-IP for apps that use it
		if clientIP := r.Header.Get("X-Forwarded-For"); clientIP != "" {
			req.Header.Set("X-Real-IP", strings.Split(clientIP, ",")[0])
		} else if r.RemoteAddr != "" {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			req.Header.Set("X-Real-IP", host)
		}
	}

	// Handle errors
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error for session %s: %v", sessionID, err)
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
	}

	// Modify response to handle redirects
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Rewrite Location header for redirects to go through the proxy
		if location := resp.Header.Get("Location"); location != "" {
			// If it's a relative path or same-origin redirect, prepend our proxy prefix
			if strings.HasPrefix(location, "/") {
				resp.Header.Set("Location", "/api/sessions/"+sessionID+"/proxy"+location)
			}
		}
		return nil
	}

	proxy.ServeHTTP(w, r)
}

// isWebSocketRequest checks if the request is a WebSocket upgrade request
func isWebSocketRequest(r *http.Request) bool {
	connectionHeader := strings.ToLower(r.Header.Get("Connection"))
	upgradeHeader := strings.ToLower(r.Header.Get("Upgrade"))
	return strings.Contains(connectionHeader, "upgrade") && upgradeHeader == "websocket"
}

// handleWebSocket handles WebSocket proxy connections
func (p *HTTPProxy) handleWebSocket(w http.ResponseWriter, r *http.Request, target *url.URL, proxyPath, sessionID string) {
	// Build the target WebSocket address
	targetAddr := target.Host
	if target.Port() == "" {
		if target.Scheme == "https" {
			targetAddr = target.Host + ":443"
		} else {
			targetAddr = target.Host + ":80"
		}
	}

	useTLS := target.Scheme == "https"

	// Connect to the backend
	rawConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		log.Printf("WebSocket proxy: failed to connect to backend %s: %v", targetAddr, err)
		http.Error(w, "Failed to connect to backend", http.StatusBadGateway)
		return
	}
	defer rawConn.Close()

	// Wrap with TLS if needed
	var backendConn net.Conn = rawConn
	if useTLS {
		tlsConn := tls.Client(rawConn, &tls.Config{
			InsecureSkipVerify: true, // Internal pod connections use self-signed certs
			ServerName:         target.Hostname(),
		})
		if err := tlsConn.Handshake(); err != nil {
			log.Printf("WebSocket proxy: TLS handshake failed: %v", err)
			http.Error(w, "TLS handshake failed", http.StatusBadGateway)
			return
		}
		backendConn = tlsConn
	}

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("WebSocket proxy: hijacking not supported")
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("WebSocket proxy: failed to hijack connection: %v", err)
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Build the WebSocket upgrade request for the backend
	r.URL.Path = proxyPath
	r.URL.RawPath = proxyPath
	r.URL.Scheme = "" // Clear scheme for the request line
	r.Host = target.Host
	r.Header.Del("Authorization")

	// Write the original request to the backend
	if err := r.Write(backendConn); err != nil {
		log.Printf("WebSocket proxy: failed to write request to backend: %v", err)
		return
	}

	// Bidirectional copy
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(backendConn, clientConn)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errCh <- err
	}()

	// Wait for either direction to complete
	<-errCh
}

// extractSessionID extracts the session ID from the proxy path
// Path format: /api/sessions/{id}/proxy/...
func extractSessionID(path string) string {
	// Remove /api/sessions/ prefix
	path = strings.TrimPrefix(path, "/api/sessions/")

	// Find the next /proxy/ segment
	idx := strings.Index(path, "/proxy")
	if idx == -1 {
		return ""
	}

	return path[:idx]
}

// stripProxyPrefix removes the /api/sessions/{id}/proxy prefix from the path
func stripProxyPrefix(path, sessionID string) string {
	prefix := "/api/sessions/" + sessionID + "/proxy"
	path = strings.TrimPrefix(path, prefix)

	// Ensure path starts with /
	if path == "" || !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return path
}
