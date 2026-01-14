package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	port    = flag.Int("port", 8080, "Proxy server port")
	verbose = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	proxy := &ProxyServer{
		verbose: *verbose,
	}

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: proxy,
	}

	printBanner()
	log.Printf("Proxy server listening on http://localhost:%d", *port)

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func printBanner() {
	fmt.Println("========================================")
	fmt.Println("       HTTP Proxy Server v1.0")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Features:")
	fmt.Println("  - HTTP/HTTPS proxy support")
	fmt.Println("  - CONNECT tunneling for HTTPS")
	fmt.Println("  - SSE/streaming support (unbuffered)")
	fmt.Println("  - Request logging")
	fmt.Println("========================================")
}

type ProxyServer struct {
	verbose bool
}

func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
	} else {
		p.handleHTTP(w, r)
	}

	log.Printf("[%s] %s %s (%v)", r.Method, r.Host, r.URL.Path, time.Since(startTime))
}

// handleConnect handles HTTPS tunneling via CONNECT method
func (p *ProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	if p.verbose {
		log.Printf("[CONNECT] Establishing tunnel to %s", r.Host)
	}

	// Connect to the target server
	targetConn, err := net.DialTimeout("tcp", r.Host, 30*time.Second)
	if err != nil {
		log.Printf("[ERROR] Failed to connect to %s: %v", r.Host, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer targetConn.Close()

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("[ERROR] Hijacking not supported")
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("[ERROR] Failed to hijack connection: %v", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	// Send 200 Connection Established
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Printf("[ERROR] Failed to send 200 response: %v", err)
		return
	}

	if p.verbose {
		log.Printf("[CONNECT] Tunnel established to %s", r.Host)
	}

	// Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(targetConn, clientConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(clientConn, targetConn)
		done <- struct{}{}
	}()

	// Wait for either direction to finish
	<-done

	if p.verbose {
		log.Printf("[CONNECT] Tunnel closed for %s", r.Host)
	}
}

// handleHTTP handles regular HTTP requests
func (p *ProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if p.verbose {
		log.Printf("[HTTP] Proxying request to %s%s", r.Host, r.URL.Path)
	}

	// Create the target URL
	targetURL := r.URL
	if !targetURL.IsAbs() {
		targetURL.Scheme = "http"
		targetURL.Host = r.Host
	}

	// Create a new request
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to create proxy request: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy headers
	copyHeaders(proxyReq.Header, r.Header)

	// Remove hop-by-hop headers
	removeHopByHopHeaders(proxyReq.Header)

	// Set X-Forwarded headers
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if prior := proxyReq.Header.Get("X-Forwarded-For"); prior != "" {
			clientIP = prior + ", " + clientIP
		}
		proxyReq.Header.Set("X-Forwarded-For", clientIP)
	}
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)
	proxyReq.Header.Set("X-Forwarded-Proto", "http")

	// Use a transport that doesn't buffer for streaming
	transport := &http.Transport{
		DisableCompression: true,
		// Don't limit idle connections for streaming
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		// Don't follow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("[ERROR] Failed to proxy request: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	copyHeaders(w.Header(), resp.Header)
	removeHopByHopHeaders(w.Header())

	// Check if this is an SSE response
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	if isSSE {
		if p.verbose {
			log.Printf("[SSE] Streaming response from %s", r.Host)
		}
		// For SSE, we need to flush after each write
		w.WriteHeader(resp.StatusCode)
		p.streamResponse(w, resp.Body)
	} else {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// streamResponse handles SSE streaming with proper flushing
func (p *ProxyServer) streamResponse(w http.ResponseWriter, body io.Reader) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("[WARN] Response writer doesn't support flushing")
		io.Copy(w, body)
		return
	}

	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			_, writeErr := w.Write(buf[:n])
			if writeErr != nil {
				return
			}
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("[ERROR] Error reading response body: %v", err)
			}
			return
		}
	}
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func removeHopByHopHeaders(header http.Header) {
	for _, h := range hopByHopHeaders {
		header.Del(h)
	}
}
