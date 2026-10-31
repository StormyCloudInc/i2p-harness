package i2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-i2p/onramp"
)

// Enabled reports whether the legacy I2P option is enabled.
// I2P destinations are routed automatically regardless of this option.
func Enabled() bool {
	return strings.EqualFold(os.Getenv("I2P_ENABLE"), "true")
}

var (
	garlicMu sync.Mutex
	garlic   *onramp.Garlic
	// Onramp mutates the shared session while dialing, so serialize calls.
	garlicDial = make(chan struct{}, 1)
)

func getGarlic() (*onramp.Garlic, error) {
	garlicMu.Lock()
	defer garlicMu.Unlock()
	if garlic == nil {
		var err error
		garlic, err = onramp.NewGarlic("pi-agent", onramp.SAM_ADDR, onramp.OPT_HUGE)
		if err != nil {
			return nil, fmt.Errorf("initialize I2P session: %w", err)
		}
	}
	return garlic, nil
}

// IsI2PHost reports whether host names an I2P destination.
func IsI2PHost(host string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSuffix(host, ".")), ".i2p")
}

// NewTransport creates an HTTP transport that routes I2P hosts through Onramp.
func NewTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = I2PDialer()
	proxy := transport.Proxy
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if IsI2PHost(req.URL.Hostname()) {
			return nil, nil
		}
		if proxy != nil {
			return proxy(req)
		}
		return nil, nil
	}
	return transport
}

// I2PDialer returns a context-aware dialer for both I2P and ordinary hosts.
func I2PDialer() func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return routeDial(ctx, network, addr, dialI2P)
	}
}

func routeDial(ctx context.Context, network, addr string, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if !IsI2PHost(host) {
		return (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, addr)
	}
	// Onramp's name lookup requires a lowercase host without a trailing dot.
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if port != "" {
		addr = net.JoinHostPort(host, port)
	} else {
		addr = host
	}
	return dial(ctx, network, addr)
}

func dialI2P(ctx context.Context, network, addr string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case garlicDial <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	slog.Info("Dialing I2P", "address", addr)
	type result struct {
		conn net.Conn
		err  error
	}
	results := make(chan result)
	go func() {
		defer func() { <-garlicDial }()
		g, err := getGarlic()
		var conn net.Conn
		if err == nil {
			// Onramp's Dial resolves .b32.i2p names; its DialContext does not.
			conn, err = g.Dial(network, addr)
		}
		select {
		case results <- result{conn: conn, err: err}:
		case <-ctx.Done():
			if conn != nil {
				conn.Close()
			}
		}
	}()
	select {
	case r := <-results:
		if r.err != nil {
			return nil, fmt.Errorf("dial I2P: %w", r.err)
		}
		if r.conn == nil {
			return nil, fmt.Errorf("dial I2P: no connection returned")
		}
		slog.Info("I2P dial successful", "address", addr)
		return r.conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
