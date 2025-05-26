package util

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestLogHTTPReqInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	log.Logger = logger

	ri := &HTTPReqInfo{
		method:    "GET",
		path:      "/test/path",
		code:      200,
		size:      1234,
		duration:  150 * time.Millisecond,
		ipAddress: net.ParseIP("127.0.0.1"),
		userAgent: "Go-http-client/1.1",
		referer:   "http://example.com",
	}

	logHTTPReqInfo(ri)

	logged := buf.String()
	tests := []struct {
		field string
		want  string
	}{
		{"method", "GET"},
		{"path", "/test/path"},
		{"code", "200"},
		{"size", "1234"},
		{"duration", "150"},
		{"ipAddress", "127.0.0.1"},
		{"userAgent", "Go-http-client/1.1"},
		{"referer", "http://example.com"},
	}

	for _, tt := range tests {
		if !strings.Contains(logged, tt.want) {
			t.Errorf("Expected log to contain %q for field %q, got: %s", tt.want, tt.field, logged)
		}
	}
}

func TestLogRequestHandler(t *testing.T) {
	tests := []struct {
		name        string
		pretty      bool
		method      string
		path        string
		userAgent   string
		referer     string
		remoteAddr  string
		wantMethod  string
		wantPath    string
		wantAgent   string
		wantReferer string
	}{
		{
			name:        "GET request with all headers",
			pretty:      false, // Only test non-pretty to avoid global logger issues
			method:      "GET",
			path:        "/api/test",
			userAgent:   "Mozilla/5.0",
			referer:     "https://example.com",
			remoteAddr:  "192.168.1.1:12345",
			wantMethod:  "GET",
			wantPath:    "/api/test",
			wantAgent:   "Mozilla/5.0",
			wantReferer: "https://example.com",
		},
		{
			name:        "POST request without headers",
			pretty:      false,
			method:      "POST",
			path:        "/api/submit",
			userAgent:   "",
			referer:     "",
			remoteAddr:  "127.0.0.1:8080",
			wantMethod:  "POST",
			wantPath:    "/api/submit",
			wantAgent:   "",
			wantReferer: "",
		},
		{
			name:        "PUT request with query parameters",
			pretty:      false,
			method:      "PUT",
			path:        "/api/update?id=123&param=value",
			userAgent:   "Go-http-client/1.1",
			referer:     "http://localhost:3000",
			remoteAddr:  "10.0.0.1:54321",
			wantMethod:  "PUT",
			wantPath:    "/api/update?id=123&param=value",
			wantAgent:   "Go-http-client/1.1",
			wantReferer: "http://localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original logger
			originalLogger := log.Logger
			defer func() {
				log.Logger = originalLogger
			}()

			// Capture log output
			var buf bytes.Buffer
			log.Logger = zerolog.New(&buf)

			// Create the logging handler
			dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("test response"))
			})

			opt := &LogRequestHandlerOptions{Pretty: tt.pretty}
			handler := LogRequestHandler(dummyHandler, opt)

			// Create test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}

			// Create response recorder
			w := httptest.NewRecorder()

			// Execute the handler
			handler.ServeHTTP(w, req)

			// Verify the response
			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			}

			if w.Body.String() != "test response" {
				t.Errorf("Expected body 'test response', got '%s'", w.Body.String())
			}

			// Verify the log output
			logged := buf.String()

			// Basic check that we got some log output
			if len(logged) == 0 {
				t.Errorf("Expected log output, got empty string")
				return
			}

			// Check that required fields are present in the log (JSON format)
			logChecks := []struct {
				field string
				want  string
			}{
				{"method", fmt.Sprintf("\"method\":\"%s\"", tt.wantMethod)},
				{"path", fmt.Sprintf("\"path\":\"%s\"", tt.wantPath)},
				{"code", "\"code\":200"},
				{"size", "\"size\":13"}, // "test response" is 13 bytes
			}

			// Only check non-empty header values
			if tt.wantAgent != "" {
				logChecks = append(logChecks, struct {
					field string
					want  string
				}{"userAgent", fmt.Sprintf("\"userAgent\":\"%s\"", tt.wantAgent)})
			}
			if tt.wantReferer != "" {
				logChecks = append(logChecks, struct {
					field string
					want  string
				}{"referer", fmt.Sprintf("\"referer\":\"%s\"", tt.wantReferer)})
			}

			for _, check := range logChecks {
				if !strings.Contains(logged, check.want) {
					t.Errorf("Expected log to contain %q for field %q, got: %s", check.want, check.field, logged)
				}
			}

			// Verify duration field exists (should be a number in JSON)
			if !strings.Contains(logged, "\"duration\":") {
				t.Errorf("Expected log to contain 'duration' field, got: %s", logged)
			}

			// Verify ipAddress field exists
			if !strings.Contains(logged, "\"ipAddress\":") {
				t.Errorf("Expected log to contain 'ipAddress' field, got: %s", logged)
			}
		})
	}
}

func TestLogRequestHandlerWithDifferentStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
	}{
		{"Not Found", http.StatusNotFound, "not found"},
		{"Internal Server Error", http.StatusInternalServerError, "error occurred"},
		{"Created", http.StatusCreated, "resource created"},
		{"No Content", http.StatusNoContent, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer

			// Save original logger
			originalLogger := log.Logger
			defer func() {
				log.Logger = originalLogger
			}()

			logger := zerolog.New(&buf)
			log.Logger = logger

			// Create a dummy handler that returns the specified status code
			dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != "" {
					w.Write([]byte(tt.response))
				}
			})

			// Create the logging handler
			opt := &LogRequestHandlerOptions{Pretty: false}
			handler := LogRequestHandler(dummyHandler, opt)

			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "127.0.0.1:12345"

			// Create response recorder
			w := httptest.NewRecorder()

			// Execute the handler
			handler.ServeHTTP(w, req)

			// Verify the response
			if w.Code != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, w.Code)
			}

			// Verify the log contains the correct status code
			logged := buf.String()
			statusCodeStr := fmt.Sprintf("\"code\":%d", tt.statusCode)
			if !strings.Contains(logged, statusCodeStr) {
				t.Errorf("Expected log to contain status code %d, got: %s", tt.statusCode, logged)
			}

			// Verify expected response size
			expectedSize := len(tt.response)
			sizeStr := fmt.Sprintf("\"size\":%d", expectedSize)
			if !strings.Contains(logged, sizeStr) {
				t.Errorf("Expected log to contain size %d, got: %s", expectedSize, logged)
			}
		})
	}
}

func TestLogRequestHandlerPrettyLogging(t *testing.T) {
	// This test verifies that the pretty option affects the logger setup
	// We can't easily test the actual pretty output format without more complex setup,
	// but we can verify the handler works with pretty enabled

	var buf bytes.Buffer

	// Save original logger
	originalLogger := log.Logger
	defer func() {
		log.Logger = originalLogger
	}()

	log.Logger = zerolog.New(&buf)

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Test with pretty logging enabled
	opt := &LogRequestHandlerOptions{Pretty: true}
	handler := LogRequestHandler(dummyHandler, opt)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify the handler still works correctly
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("Expected body 'ok', got '%s'", w.Body.String())
	}
}
