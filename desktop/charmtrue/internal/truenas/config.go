package truenas

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAPIPath     = "/api/current"
	defaultEventBuffer = 64
	defaultConcurrency = 16
	defaultReadLimit   = 16 << 20 // 16 MiB
	defaultRetryLimit  = 3
	defaultRetryDelay  = 100 * time.Millisecond
	defaultReconnect   = time.Second
)

// Config configures a TrueNAS 25.10 JSON-RPC WebSocket connection.
type Config struct {
	// Endpoint accepts a host name, an HTTP(S) URL, or a WS(S) URL. When the
	// URL has no path, /api/current is added automatically.
	Endpoint string
	Username string
	APIKey   string
	Password string
	// OTP completes a PASSWORD_PLAIN login that returns OTP_REQUIRED.
	OTP string

	// InsecureSkipVerify accepts TLS certificates that cannot be verified by
	// the operating system. Use only for explicitly trusted private systems.
	InsecureSkipVerify bool

	// HTTPClient can provide a custom transport, for example one that trusts a
	// private CA used by a TrueNAS appliance.
	HTTPClient *http.Client

	// EventBuffer controls the number of notifications retained for consumers.
	// A value smaller than one uses the default.
	EventBuffer int

	// ReadLimit is the maximum size in bytes of one WebSocket message received
	// from TrueNAS. Zero selects the 16 MiB default.
	ReadLimit int64

	// MaxConcurrentCalls limits in-flight RPC calls. BusyRetryLimit and
	// BusyRetryDelay control retries for the TrueNAS -32000 overload error.
	// Zero selects the default retry limit; a negative value disables retries.
	MaxConcurrentCalls int
	BusyRetryLimit     int
	BusyRetryDelay     time.Duration

	// ReconnectDelay controls automatic reconnect attempts after an established
	// connection is lost. DisableReconnect disables background attempts; a later
	// Call can still reconnect and restore subscriptions on demand.
	ReconnectDelay   time.Duration
	DisableReconnect bool
}

func (c Config) validate() (Config, error) {
	endpoint, err := normalizeEndpoint(c.Endpoint)
	if err != nil {
		return Config{}, err
	}

	c.Endpoint = endpoint
	c.Username = strings.TrimSpace(c.Username)
	if c.APIKey != "" && c.Password != "" {
		return Config{}, &ValidationError{Field: "credentials", Message: "API key and password cannot be used together"}
	}
	if c.OTP != "" && c.Password == "" {
		return Config{}, &ValidationError{Field: "OTP", Message: "requires password authentication"}
	}
	if (c.APIKey != "" || c.Password != "") && c.Username == "" {
		return Config{}, &ValidationError{Field: "username", Message: "is required for authentication"}
	}
	if c.InsecureSkipVerify {
		if c.HTTPClient != nil {
			return Config{}, &ValidationError{Field: "HTTP client", Message: "cannot be combined with insecure TLS"}
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Explicit opt-in for private TrueNAS certificates.
		c.HTTPClient = &http.Client{Transport: transport}
	}
	if c.EventBuffer < 1 {
		c.EventBuffer = defaultEventBuffer
	}
	if c.ReadLimit < 0 {
		return Config{}, &ValidationError{Field: "read limit", Message: "cannot be negative"}
	}
	if c.ReadLimit == 0 {
		c.ReadLimit = defaultReadLimit
	}
	if c.MaxConcurrentCalls < 1 {
		c.MaxConcurrentCalls = defaultConcurrency
	}
	if c.BusyRetryLimit < 0 {
		c.BusyRetryLimit = 0
	} else if c.BusyRetryLimit == 0 {
		c.BusyRetryLimit = defaultRetryLimit
	}
	if c.BusyRetryDelay <= 0 {
		c.BusyRetryDelay = defaultRetryDelay
	}
	if c.ReconnectDelay <= 0 {
		c.ReconnectDelay = defaultReconnect
	}
	return c, nil
}

func normalizeEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", &ValidationError{Field: "endpoint", Message: "is required"}
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "wss://" + endpoint
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", &ValidationError{Field: "endpoint", Message: err.Error()}
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "ws", "wss":
		u.Scheme = strings.ToLower(u.Scheme)
	default:
		return "", &ValidationError{Field: "endpoint scheme", Message: fmt.Sprintf("%q is not supported", u.Scheme)}
	}
	if u.Host == "" {
		return "", &ValidationError{Field: "endpoint host", Message: "is required"}
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", &ValidationError{Field: "endpoint", Message: "must not contain a query or fragment"}
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = defaultAPIPath
	}
	return u.String(), nil
}
