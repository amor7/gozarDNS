// Package certs provides the certificates both roles serve: a real one from
// Let's Encrypt when a domain name is configured, and a self-signed one for
// everything else.
//
// Certificates are obtained with the TLS-ALPN-01 challenge on port 443, which
// the role's own port-443 listener hands over when it sees the domain in the
// ClientHello. Port 80 is never borrowed, so nothing that travels over it -
// console downloads above all - ever pauses for a renewal.
package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// SelfSigned loads the certificate stored as <dir>/<name>.crt/.key, creating
// a ten-year one for hosts when there is none.
func SelfSigned(dir, name string, hosts []string) (tls.Certificate, error) {
	crt := filepath.Join(dir, name+".crt")
	key := filepath.Join(dir, name+".key")
	if c, err := tls.LoadX509KeyPair(crt, key); err == nil {
		return c, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, err
	}
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "gozar " + name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else if h != "" {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &pk.PublicKey, pk)
	if err != nil {
		return tls.Certificate{}, err
	}
	kb, err := x509.MarshalECPrivateKey(pk)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(crt, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(crt, key)
}

// Fingerprint is the hex SHA-256 of a certificate's leaf, as gates pin it.
func Fingerprint(c tls.Certificate) string {
	if len(c.Certificate) == 0 {
		return ""
	}
	s := sha256.Sum256(c.Certificate[0])
	return hex.EncodeToString(s[:])
}

// PinnedConfig trusts exactly one certificate, by fingerprint, and nothing
// else - no CA, no hostname. What a gate uses to reach its hub.
func PinnedConfig(fingerprint string) *tls.Config {
	want := strings.ToLower(fingerprint)
	return &tls.Config{
		InsecureSkipVerify: true, // replaced by the pin check below
		MinVersion:         tls.VersionTLS12,
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("hub presented no certificate")
			}
			s := sha256.Sum256(raw[0])
			if hex.EncodeToString(s[:]) != want {
				return errors.New("hub certificate does not match the pinned fingerprint")
			}
			return nil
		},
	}
}

// Server bundles a domain certificate (when there is a domain) with a
// self-signed fallback, and knows how to answer ACME challenges.
type Server struct {
	Domain   string
	manager  *autocert.Manager
	fallback tls.Certificate
}

// NewServer prepares certificates for domain (may be empty). cacheDir keeps
// issued certificates across restarts.
func NewServer(domain, email, cacheDir string, fallback tls.Certificate) *Server {
	s := &Server{Domain: strings.ToLower(strings.TrimSpace(domain)), fallback: fallback}
	if s.Domain != "" {
		s.manager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(cacheDir),
			HostPolicy: autocert.HostWhitelist(s.Domain),
			Email:      email,
		}
	}
	return s
}

// TLSConfig serves the domain certificate for the domain and the fallback
// for anything else (a bare IP, most often).
func (s *Server) TLSConfig() *tls.Config {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if s.manager != nil && strings.EqualFold(h.ServerName, s.Domain) {
				c, err := s.manager.GetCertificate(h)
				if err == nil {
					return c, nil
				}
				if isChallenge(h) {
					return nil, err
				}
			}
			return &s.fallback, nil
		},
	}
	if s.manager != nil {
		cfg.NextProtos = append(cfg.NextProtos, acme.ALPNProto)
	}
	return cfg
}

func isChallenge(h *tls.ClientHelloInfo) bool {
	for _, p := range h.SupportedProtos {
		if p == acme.ALPNProto {
			return true
		}
	}
	return false
}

// Warm asks for the domain certificate in the background so the first
// visitor does not wait for issuance.
func (s *Server) Warm() {
	if s.manager == nil {
		return
	}
	go func() {
		time.Sleep(3 * time.Second)
		_, _ = s.manager.GetCertificate(&tls.ClientHelloInfo{ServerName: s.Domain, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}})
	}()
}

// Status reports whether a real certificate is in hand, and until when.
func (s *Server) Status() (issued bool, notAfter time.Time) {
	if s.manager == nil {
		return false, time.Time{}
	}
	// Read the cache rather than asking the manager, which would start an
	// issuance and block on it.
	data, err := s.manager.Cache.Get(context.Background(), s.Domain)
	if err != nil {
		return false, time.Time{}
	}
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			return false, time.Time{}
		}
		if b.Type == "CERTIFICATE" {
			c, err := x509.ParseCertificate(b.Bytes)
			if err != nil {
				return false, time.Time{}
			}
			return time.Now().Before(c.NotAfter), c.NotAfter
		}
	}
}
