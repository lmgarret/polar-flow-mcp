package flow

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// newBrowserTransport returns an http.RoundTripper that performs the TLS
// handshake using uTLS with a Chrome ClientHello and speaks HTTP/2 — the
// combination CloudFront's bot ruleset accepts on flow.polar.com.
//
// We use http2.Transport directly (instead of http.Transport + uTLS via
// DialTLSContext) because net/http's HTTP/2 upgrade path type-asserts the
// dialed conn to *tls.Conn, which a *utls.UConn is not. Bypassing
// http.Transport's h2 detection sidesteps that problem entirely.
//
// Trade-off: if Polar ever serves a host that only speaks HTTP/1.1 we'll
// fail the connection. Both flow.polar.com and auth.polar.com support h2
// today.
func newBrowserTransport() (http.RoundTripper, error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http2.Transport{ //nolint:staticcheck // http.Transport can't be handed a *utls.UConn (see comment above)
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			raw, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			// Advertise the same ALPN list Chrome does so the JA4 fingerprint
			// matches a real browser.
			cfg := &utls.Config{
				ServerName: host,
				NextProtos: []string{"h2", "http/1.1"},
			}
			u := utls.UClient(raw, cfg, utls.HelloChrome_Auto)
			if err := u.HandshakeContext(ctx); err != nil {
				_ = raw.Close()
				return nil, err
			}
			cs := u.ConnectionState()
			slog.Debug("flow: utls handshake",
				"host", host,
				"alpn", cs.NegotiatedProtocol,
				"version", cs.Version,
				"cipher", cs.CipherSuite,
			)
			if cs.NegotiatedProtocol != "h2" {
				_ = u.Close()
				return nil, fmt.Errorf("flow: expected h2 ALPN, got %q", cs.NegotiatedProtocol)
			}
			return u, nil
		},
	}, nil
}
