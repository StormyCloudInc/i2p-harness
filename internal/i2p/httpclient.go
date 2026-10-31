package i2p

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// I2PClient wraps a standard http.Client with I2P support.
type I2PClient struct {
	Transport http.RoundTripper
}

// NewI2PClient creates a new HTTP client that routes I2P hosts through Onramp.
func NewI2PClient() *http.Client {
	return &http.Client{
		Transport: NewTransport(),
		Timeout:   30 * time.Second,
	}
}

// i2pTransport wraps http.RoundTripper and uses I2P dialer for .i2p addresses.
type i2pTransport struct {
	Transport http.RoundTripper
	dialer    func(context.Context, string, string) (net.Conn, error)
}

// buildHeaders converts an http.Request to raw HTTP headers
func buildHeaders(req *http.Request) string {
	var buf bytes.Buffer

	// Add request line
	buf.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI()))

	// Add headers
	for k, v := range req.Header {
		for _, h := range v {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", k, h))
		}
	}

	// Add host header if not present
	if _, ok := req.Header["Host"]; !ok {
		buf.WriteString(fmt.Sprintf("Host: %s\r\n", req.URL.Host))
	}

	return buf.String()
}

// readHTTPResponse reads and parses an HTTP response from a connection
func readHTTPResponse(conn net.Conn) (*http.Response, error) {
	// Set a timeout for reading
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// Read status line
	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	responseData := buf[:n]

	// Parse status line
	statusEnd := bytes.Index(responseData, []byte("\r\n"))
	if statusEnd == -1 {
		return nil, fmt.Errorf("invalid response: %s", string(responseData))
	}

	statusLine := string(responseData[:statusEnd])
	parts := strings.Split(statusLine, " ")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid HTTP status line: %s", statusLine)
	}

	statusCodeStr := parts[1]
	statusCode := 200 // Default
	if len(parts) >= 3 {
		statusCodeStr = parts[1] + " " + parts[2]
	} else if len(parts) == 2 {
		statusCode = 200
	} else {
		for i := 2; i < len(parts); i++ {
			statusCodeStr += " " + parts[i]
		}
	}

	// Find body start
	bodyStart := bytes.Index(responseData, []byte("\r\n\r\n"))
	if bodyStart == -1 {
		bodyStart = len(responseData)
	} else {
		bodyStart += 4
	}

	// Parse headers
	headersStart := bodyStart
	headersEnd := len(responseData)
	for i := bodyStart; i < len(responseData); {
		lineEnd := bytes.Index(responseData[i:], []byte("\r\n"))
		if lineEnd == -1 {
			break
		}
		lineEnd += i
		if bytes.HasPrefix(responseData[i:lineEnd], []byte("\r\n")) {
			headersEnd = i
			break
		}
		i = lineEnd + 2
	}

	headersText := responseData[headersStart:headersEnd]
	responseHeaders := make(http.Header)

	lines := strings.Split(string(headersText), "\r\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 2 {
			responseHeaders.Add(strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}

	// Build response
	responseBody := responseData[headersEnd:]

	// Parse status code
	switch statusCodeStr {
	case "200":
		statusCode = http.StatusOK
	case "201":
		statusCode = http.StatusCreated
	case "204":
		statusCode = http.StatusNoContent
	case "301":
		statusCode = http.StatusMovedPermanently
	case "302":
		statusCode = http.StatusFound
	case "304":
		statusCode = http.StatusNotModified
	case "400":
		statusCode = http.StatusBadRequest
	case "401":
		statusCode = http.StatusUnauthorized
	case "403":
		statusCode = http.StatusForbidden
	case "404":
		statusCode = http.StatusNotFound
	case "500":
		statusCode = http.StatusInternalServerError
	case "502":
		statusCode = http.StatusBadGateway
	case "503":
		statusCode = http.StatusServiceUnavailable
	default:
		statusCode = http.StatusOK
	}

	return &http.Response{
		Status:        statusCodeStr,
		StatusCode:    statusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        responseHeaders,
		Body:          io.NopCloser(bytes.NewReader(responseBody)),
		ContentLength: int64(len(responseBody)),
	}, nil
}

// RoundTrip implements http.RoundTripper.
func (t *i2pTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.Contains(req.URL.Host, ".i2p") {
		fmt.Printf("[I2P] ➡️  Standard request: %s %s\n", req.Method, req.URL.String())
		return t.Transport.RoundTrip(req)
	}

	fmt.Printf("[I2P] 🔌 I2P request: %s %s\n", req.Method, req.URL.String())

	// Get I2P dialer
	if t.dialer == nil {
		t.dialer = I2PDialer()
	}

	// Establish I2P connection
	conn, err := t.dialer(context.Background(), "tcp", req.URL.Host)
	if err != nil {
		fmt.Printf("[I2P] ❌ Dial failed for %s: %v\n", req.URL.Host, err)
		return nil, err
	}
	defer conn.Close()

	// Build HTTP request manually
	headers := buildHeaders(req)

	// Send request
	fmt.Printf("[I2P] 📤 Sending request...\n")
	fullRequest := headers + "\r\n"
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		fullRequest += string(body)
	}

	_, err = conn.Write([]byte(fullRequest))
	if err != nil {
		return nil, fmt.Errorf("failed to send I2P request: %w", err)
	}
	fmt.Printf("[I2P] 📤 Request sent successfully\n")

	// Read response
	fmt.Printf("[I2P] 📥 Reading response...\n")
	response, err := readHTTPResponse(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read I2P response: %w", err)
	}

	fmt.Printf("[I2P] ✅ I2P request completed: %s - Status: %d\n", req.Method, response.StatusCode)

	return response, nil
}
