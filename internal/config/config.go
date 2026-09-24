// Package config reads and writes /etc/gozar/config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultPath = "/etc/gozar/config.json"

// DefaultHubSNI is announced to the hub when the hub has no domain of its own.
const DefaultHubSNI = "example.com"

type Role string

const (
	RoleHub  Role = "hub"
	RoleGate Role = "gate"
)

type Config struct {
	Role    Role   `json:"role"`
	DataDir string `json:"data_dir"`
	Hub     *Hub   `json:"hub,omitempty"`
	Gate    *Gate  `json:"gate,omitempty"`

	path string
}

// Hub is the machine outside the filtered network. It keeps the database,
// serves the admin console and carries traffic out to the real services.
type Hub struct {
	PublicIP    string `json:"public_ip"`
	AdminDomain string `json:"admin_domain,omitempty"`
	AdminPort   int    `json:"admin_port"`
	ControlPort int    `json:"control_port"`
	ACMEEmail   string `json:"acme_email,omitempty"`
	// "auto", "on" or "off": send Google's names out over IPv6 when the
	// machine has it. Google refuses some services to IPv4 ranges it
	// geolocates badly.
	IPv6Egress string `json:"ipv6_egress"`
	HTTPSPort  int    `json:"https_port"`
	HTTPPort   int    `json:"http_port"`
}

// Gate is the machine customers point their DNS at.
type Gate struct {
	HubAddr        string `json:"hub_addr"`
	HubControlPort int    `json:"hub_control_port"`
	HubHTTPSPort   int    `json:"hub_https_port"`
	HubHTTPPort    int    `json:"hub_http_port"`
	Fingerprint    string `json:"fingerprint"`
	// HubSNI is the server name the gate announces when it talks to the hub.
	// Nothing checks it; it only has to be present (see certs.PinnedConfig).
	HubSNI         string   `json:"hub_sni,omitempty"`
	GateID         string   `json:"gate_id"`
	Secret         string   `json:"secret"`
	PublicIP       string   `json:"public_ip"`
	PortalDomain   string   `json:"portal_domain,omitempty"`
	PortalPort     int      `json:"portal_port"`
	ACMEEmail      string   `json:"acme_email,omitempty"`
	DNSListen      string   `json:"dns_listen"`
	HTTPSListen    string   `json:"https_listen"`
	HTTPListen     string   `json:"http_listen"`
	STUNListen     string   `json:"stun_listen,omitempty"`
	Upstreams      []string `json:"upstreams,omitempty"`
	CleanUpstreams []string `json:"clean_upstreams,omitempty"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s not found - run `gozar setup` first", path)
		}
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.path = path
	c.applyDefaults()
	return c, c.validate()
}

func (c *Config) applyDefaults() {
	if c.DataDir == "" {
		c.DataDir = "/var/lib/gozar"
	}
	if h := c.Hub; h != nil {
		def(&h.AdminPort, 2053)
		def(&h.ControlPort, 7443)
		def(&h.HTTPSPort, 443)
		def(&h.HTTPPort, 80)
		if h.IPv6Egress == "" {
			h.IPv6Egress = "auto"
		}
	}
	if g := c.Gate; g != nil {
		def(&g.HubControlPort, 7443)
		def(&g.HubHTTPSPort, 443)
		def(&g.HubHTTPPort, 80)
		def(&g.PortalPort, 8443)
		defs(&g.HubSNI, DefaultHubSNI)
		defs(&g.DNSListen, ":53")
		defs(&g.HTTPSListen, ":443")
		defs(&g.HTTPListen, ":80")
		if len(g.CleanUpstreams) == 0 {
			g.CleanUpstreams = []string{"1.1.1.1", "8.8.8.8"}
		}
	}
}

func def(v *int, d int) {
	if *v == 0 {
		*v = d
	}
}

func defs(v *string, d string) {
	if *v == "" {
		*v = d
	}
}

func (c *Config) validate() error {
	switch c.Role {
	case RoleHub:
		if c.Hub == nil {
			return errors.New("config: role is hub but the hub section is missing")
		}
	case RoleGate:
		g := c.Gate
		if g == nil {
			return errors.New("config: role is gate but the gate section is missing")
		}
		if g.HubAddr == "" || g.GateID == "" || g.Secret == "" || g.Fingerprint == "" {
			return errors.New("config: gate is not paired with a hub - run `gozar setup`")
		}
	default:
		return fmt.Errorf("config: unknown role %q", c.Role)
	}
	return nil
}

func (c *Config) Path() string { return c.path }

// Save writes the file atomically with owner-only permissions: it holds secrets.
func (c *Config) Save(path string) error {
	if path == "" {
		path = c.path
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	c.path = path
	return os.Rename(tmp, path)
}
