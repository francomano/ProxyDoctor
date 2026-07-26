package check

import "time"

// CheckCategory represents the category of a check
type CheckCategory string

const (
	CategoryLeakDetection CheckCategory = "leak"
	CategoryProtocol      CheckCategory = "protocol"
	CategoryTLS           CheckCategory = "tls"
	CategoryNetwork       CheckCategory = "network"
	CategoryGeolocation   CheckCategory = "geolocation"
	CategoryReputation    CheckCategory = "reputation"
	CategoryBrowser       CheckCategory = "browser"
)

// Status represents the result status of a check
type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
	StatusError   Status = "error"
)

// Severity represents the severity level of a finding
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// CheckResult contains the standardized result of a check
type CheckResult struct {
	ID               string                 `json:"id"`
	Category         CheckCategory          `json:"category"`
	Status           Status                 `json:"status"`
	Severity         Severity               `json:"severity"`
	Confidence       float64                `json:"confidence"`                  // 0.0-1.0
	Evidence         map[string]interface{} `json:"evidence"`                    // Raw collected data
	Explanation      string                 `json:"explanation"`                 // Human-readable
	ProbableCauses   []string               `json:"probable_causes,omitempty"`   // Why did it fail?
	SuggestedActions []string               `json:"suggested_actions,omitempty"` // What to do?
	References       []string               `json:"references,omitempty"`        // URLs to docs
	ExecutionTime    time.Duration          `json:"execution_time"`
	Timestamp        time.Time              `json:"timestamp"`
}

// Checker is the interface every check must implement
type Checker interface {
	// Metadata
	ID() string
	Name() string
	Description() string
	Category() CheckCategory

	// Dependencies
	DependsOn() []string // IDs of checks this depends on

	// Execution
	Execute(ctx ExecutionContext) CheckResult
}

// ExecutionContext provides access to adapters and shared data during check execution
type ExecutionContext interface {
	// Input data
	GetURL() string
	GetProxyConfig() ProxyConfig

	// Network adapters
	GetDirectAdapter() NetworkAdapter
	GetProxyAdapter() NetworkAdapter

	// Shared data between checks
	GetSharedData(key string) interface{}
	SetSharedData(key string, value interface{})

	// Control
	GetTimeout() time.Duration
	IsCancelled() bool
}

// ProxyConfig represents proxy configuration
type ProxyConfig struct {
	Type     ProxyType
	Host     string
	Port     int
	Username string
	Password string
}

// ProxyType represents the type of proxy
type ProxyType string

const (
	ProxyTypeDirect ProxyType = "direct"
	ProxyTypeHTTP   ProxyType = "http"
	ProxyTypeHTTPS  ProxyType = "https"
	ProxyTypeSOCKS4 ProxyType = "socks4"
	ProxyTypeSOCKS5 ProxyType = "socks5"
)

// NetworkAdapter abstracts different network connection types
type NetworkAdapter interface {
	// Identification
	Type() ProxyType

	// HTTP requests
	ExecuteHTTPRequest(req *HTTPRequest) (*HTTPResponse, error)
	FollowRedirects(url string, maxRedirects int) ([]RedirectStep, error)

	// DNS
	ResolveDNS(hostname string) ([]string, error) // Returns IPs as strings

	// Network info
	GetPublicIP() (string, error)
	TestPort(host string, port int, timeout time.Duration) (bool, error)

	// TLS
	GetTLSCertificate(hostname string) (*CertificateInfo, error)
	GetTLSCipherSuite(hostname string) (string, error)
	GetTLSVersion(hostname string) (string, error)
}

// HTTPRequest represents an HTTP request
type HTTPRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
	Duration   time.Duration
}

// RedirectStep represents a single redirect in a chain
type RedirectStep struct {
	From       string
	To         string
	StatusCode int
	Headers    map[string][]string
}

// CertificateInfo represents TLS certificate information
type CertificateInfo struct {
	Subject            string
	Issuer             string
	NotBefore          time.Time
	NotAfter           time.Time
	SANs               []string // Subject Alternative Names
	SignatureAlgorithm string
	PublicKeyType      string
	PublicKeyBits      int
	IsValid            bool
	ValidationError    string // If not valid
}
