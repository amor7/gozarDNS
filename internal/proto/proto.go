// Package proto is the conversation between a gate and its hub.
//
// The gate always dials the hub, never the other way round: the gate sits in
// the harder network position and should never need an inbound port opened
// for the hub's sake.
package proto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/amor7/gozarDNS/internal/rules"
	"github.com/amor7/gozarDNS/internal/sysmetrics"
)

// Paths on the hub's control port.
const (
	PathSync   = "/v1/sync"
	PathPortal = "/v1/portal/" // + the portal API path
	PathHello  = "/v1/hello"
)

// Headers the gate sets on portal calls it forwards. The hub trusts them only
// on a request that carries a valid gate credential.
const (
	HeaderClientIP = "X-Gozar-Client-Ip"
	HeaderGateHost = "X-Gozar-Portal-Host"
	HeaderGateIP   = "X-Gozar-Gate-Ip"
	HeaderTarget   = "X-Gozar-Target" // host:port a tunnel stream is for
)

type SyncRequest struct {
	// Batch makes usage idempotent: a batch the hub has already folded in is
	// acknowledged without being counted twice, so a reply lost on the way
	// back cannot double-bill anyone.
	Batch    string            `json:"batch"`
	Usage    map[int64]Usage   `json:"usage"`
	Seen     map[string]int64  `json:"seen,omitempty"` // client ip -> unix time of last activity
	Host     sysmetrics.Sample `json:"host"`
	Version  string            `json:"version"`
	PublicIP string            `json:"public_ip"`
	Have     string            `json:"have"` // config revision the gate already holds
	Stats    GateStats         `json:"stats"`
}

type Usage struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type GateStats struct {
	Queries   int64  `json:"queries"` // since the previous sync
	Routed    int64  `json:"routed"`
	Refused   int64  `json:"refused"`
	OpenConns int64  `json:"open_conns"`
	Tunnel    string `json:"tunnel,omitempty"` // "", "up" or "down"
}

type SyncResponse struct {
	Revision string      `json:"revision"`
	Config   *GateConfig `json:"config,omitempty"` // only when Revision differs from Have
	Interval int         `json:"interval"`         // seconds until the next sync
}

// GateConfig is everything a gate needs to serve customers on its own.
type GateConfig struct {
	// Enforce limits DNS and proxying to registered addresses. When false,
	// everybody is served with the default plan - how a fresh install works
	// before the first customer exists.
	Enforce bool `json:"enforce"`
	// Tunnel carries customer connections to the hub inside one HTTP/2
	// connection on port 443 instead of one plain TCP connection each.
	Tunnel      bool                 `json:"tunnel,omitempty"`
	DefaultPlan int64                `json:"default_plan"`
	Clients     []Client             `json:"clients"`
	Plans       map[int64]rules.Spec `json:"plans"`
	Brand       string               `json:"brand"`
}

type Client struct {
	IP   string `json:"ip"`
	User int64  `json:"user"`
	Name string `json:"name"`
	Plan int64  `json:"plan"`
	Kbps int    `json:"kbps"`
}

// ---------------------------------------------------------------- join token

// JoinToken is what the hub prints for a new gate: everything the gate needs
// to find the hub, recognise it, and prove who it is.
type JoinToken struct {
	Hub         string `json:"h"`
	ControlPort int    `json:"c"`
	HTTPSPort   int    `json:"s,omitempty"`
	HTTPPort    int    `json:"p,omitempty"`
	Fingerprint string `json:"f"`
	SNI         string `json:"n,omitempty"`
	GateID      string `json:"g"`
	Secret      string `json:"k"`
}

const tokenPrefix = "gz1_"

func (t JoinToken) Encode() string {
	b, _ := json.Marshal(t)
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
}

func ParseJoinToken(s string) (JoinToken, error) {
	var t JoinToken
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, tokenPrefix) {
		return t, errors.New("this is not a Gozar join token (it should start with gz1_)")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, tokenPrefix))
	if err != nil {
		return t, errors.New("the join token is damaged - copy it again, all of it")
	}
	if err := json.Unmarshal(b, &t); err != nil || t.Hub == "" || t.GateID == "" || t.Secret == "" || t.Fingerprint == "" {
		return t, errors.New("the join token is incomplete - copy it again, all of it")
	}
	return t, nil
}

// PayOrder is what the hub tells a gate about an online payment. It carries
// the gateway credential, so it only ever moves over the gate's
// authenticated channel.
type PayOrder struct {
	OrderID    string `json:"order_id"`
	Gateway    string `json:"gateway"`
	AmountRial int64  `json:"amount_rial"`
	Name       string `json:"name,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Desc       string `json:"desc,omitempty"`
	Key        string `json:"key"`
	Sandbox    bool   `json:"sandbox"`
	Ref        string `json:"ref,omitempty"`
	Status     string `json:"status,omitempty"`
}
