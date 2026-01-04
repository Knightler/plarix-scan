// Package proxy implements the HTTP forward proxy for LLM API interception.
//
// Purpose: Route LLM API calls through local proxy to extract usage data.
// Public API: Server, Config, Handler
// Usage: Create Server with Config, call Start() to begin listening.
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"plarix-action/internal/ledger"
	"plarix-action/internal/providers/anthropic"
	"plarix-action/internal/providers/openai"
	"plarix-action/internal/providers/openrouter"
)

// Config holds proxy configuration.
type Config struct {
	Providers            []string           // e.g., ["openai", "anthropic", "openrouter"]
	OnEntry              func(ledger.Entry) // Callback for each recorded entry
	StreamUsageInjection bool               // Opt-in for OpenAI stream usage injection
}

// Server is the HTTP forward proxy server.
type Server struct {
	config     Config
	listener   net.Listener
	httpServer *http.Server
	mu         sync.Mutex
	started    bool
}

// providerTargets maps provider names to their API base URLs.
var providerTargets = map[string]string{
	"openai":     "https://api.openai.com",
	"anthropic":  "https://api.anthropic.com",
	"openrouter": "https://openrouter.ai",
}

// NewServer creates a new proxy server.
func NewServer(config Config) *Server {
	s := &Server{config: config}
	s.httpServer = &http.Server{
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Start begins listening on a random available port on localhost.
// Returns the port number.
func (s *Server) Start() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return 0, fmt.Errorf("server already started")
	}

	var err error
	// Always bind to localhost for Action security
	s.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen: %w", err)
	}

	port := s.listener.Addr().(*net.TCPAddr).Port
	s.started = true

	go s.httpServer.Serve(s.listener)

	return port, nil
}

// Stop shuts down the server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}
	s.started = false
	return s.httpServer.Close()
}

// Port returns the listening port, or 0 if not started.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return 0
	}
	return s.listener.Addr().(*net.TCPAddr).Port
}

// ServeHTTP handles incoming proxy requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract provider from path prefix: /openai/v1/... -> openai
	pathParts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(pathParts) < 1 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	provider := pathParts[0]
	targetBase, ok := providerTargets[provider]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown provider: %s", provider), http.StatusBadRequest)
		return
	}

	// Reconstruct target path
	targetPath := "/"
	if len(pathParts) > 1 {
		targetPath = "/" + pathParts[1]
	}

	targetURL, _ := url.Parse(targetBase)

	// Check for environment variable override (TEST_UPSTREAM_*)
	// Format: PLARIX_UPSTREAM_OPENAI, PLARIX_UPSTREAM_ANTHROPIC
	envParam := fmt.Sprintf("PLARIX_UPSTREAM_%s", strings.ToUpper(provider))
	if override := strings.TrimSpace(os.Getenv(envParam)); override != "" {
		if parsed, err := url.Parse(override); err == nil {
			targetURL = parsed
		}
	}

	// Optionally inject stream_options for OpenAI (opt-in only)
	if s.config.StreamUsageInjection && provider == "openai" {
		s.injectStreamOptions(r)
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = targetPath
			req.Host = targetURL.Host
		},
		ModifyResponse: func(resp *http.Response) error {
			return s.handleResponse(provider, targetPath, resp)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
}

// injectStreamOptions modifies OpenAI requests to include stream_options for usage reporting.
// Only called when StreamUsageInjection is true.
func (s *Server) injectStreamOptions(r *http.Request) {
	if r.Body == nil || r.ContentLength == 0 {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	r.Body.Close()

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		// Not JSON, restore body and continue
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return
	}

	// Only inject if stream=true and stream_options not already set
	if stream, ok := payload["stream"].(bool); ok && stream {
		if _, exists := payload["stream_options"]; !exists {
			payload["stream_options"] = map[string]interface{}{
				"include_usage": true,
			}
		}
	}

	modified, err := json.Marshal(payload)
	if err != nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(modified))
	r.ContentLength = int64(len(modified))
}

// handleResponse processes the API response to extract usage data.
func (s *Server) handleResponse(provider, endpoint string, resp *http.Response) error {
	// Only process successful responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	contentType := resp.Header.Get("Content-Type")

	// Detect streaming responses (SSE)
	isStreaming := strings.Contains(contentType, "text/event-stream")

	if isStreaming {
		// Wrap body to intercept usage
		interceptor := newStreamInterceptor(resp.Body, provider, endpoint, func(e ledger.Entry) {
			if s.config.OnEntry != nil {
				s.config.OnEntry(e)
			}
		})
		resp.Body = interceptor
		return nil
	}

	// Non-streaming: only process JSON
	if !strings.Contains(contentType, "application/json") {
		return nil
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil // Don't fail the request if we can't read
	}
	resp.Body.Close()

	// Create new reader for the client
	resp.Body = io.NopCloser(strings.NewReader(string(body)))
	resp.ContentLength = int64(len(body))

	// Parse usage based on provider
	entry := s.parseUsage(provider, endpoint, body)
	if s.config.OnEntry != nil {
		s.config.OnEntry(entry)
	}

	return nil
}

// parseUsage extracts usage data from the response body.
func (s *Server) parseUsage(provider, endpoint string, body []byte) ledger.Entry {
	entry := ledger.Entry{
		Provider:  provider,
		Endpoint:  endpoint,
		Streaming: false,
	}

	switch provider {
	case "openai":
		openai.ParseResponse(body, &entry)
	case "anthropic":
		anthropic.ParseResponse(body, &entry)
	case "openrouter":
		openrouter.ParseResponse(body, &entry)
	default:
		entry.CostKnown = false
		entry.UnknownReason = "unsupported provider"
	}

	return entry
}
