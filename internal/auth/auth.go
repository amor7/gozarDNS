// Package auth hashes passwords, mints session tokens and checks TOTP codes.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters: 64 MiB, 2 passes. Around 50 ms on a small VPS.
const (
	argTime    = 2
	argMemory  = 64 * 1024
	argThreads = 2
	argKeyLen  = 32
)

// HashPassword returns a self-describing argon2id hash.
func HashPassword(pw string) string {
	salt := RandomBytes(16)
	key := argon2.IDKey([]byte(pw), salt, argTime, argMemory, argThreads, argKeyLen)
	return fmt.Sprintf("argon2id$%d$%d$%d$%s$%s", argTime, argMemory, argThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// CheckPassword compares in constant time. A malformed hash never matches.
func CheckPassword(hash, pw string) bool {
	p := strings.Split(hash, "$")
	if len(p) != 6 || p[0] != "argon2id" {
		return false
	}
	t, err1 := strconv.ParseUint(p[1], 10, 32)
	m, err2 := strconv.ParseUint(p[2], 10, 32)
	th, err3 := strconv.ParseUint(p[3], 10, 8)
	salt, err4 := base64.RawStdEncoding.DecodeString(p[4])
	want, err5 := base64.RawStdEncoding.DecodeString(p[5])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || m > 1<<20 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, uint32(t), uint32(m), uint8(th), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash lets a login for an unknown name cost the same as a real one.
var dummyHash = HashPassword("gozar-timing-equaliser")

func BurnTime(pw string) { CheckPassword(dummyHash, pw) }

func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// Token returns a URL-safe random string with n bytes of entropy.
func Token(n int) string { return base64.RawURLEncoding.EncodeToString(RandomBytes(n)) }

// HashToken is how session tokens are stored: a database leak must not hand
// out live sessions.
func HashToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

// Password generates a readable password without look-alike characters.
func Password(n int) string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := RandomBytes(n)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// ---------------------------------------------------------------- TOTP

func NewTOTPSecret() string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(RandomBytes(20))
}

func TOTPURI(secret, issuer, account string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&digits=6&period=30",
		escape(issuer), escape(account), secret, escape(issuer))
}

func escape(s string) string {
	return strings.NewReplacer(" ", "%20", ":", "%3A", "&", "%26", "?", "%3F", "/", "%2F").Replace(s)
}

func totpAt(secret string, counter uint64) (string, bool) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil {
		return "", false
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", code), true
}

// usedCodes stops a code being replayed inside its own window.
var usedCodes sync.Map

// CheckTOTP accepts the current code and one step either side for clock skew.
func CheckTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	now := uint64(time.Now().Unix() / 30)
	for _, c := range []uint64{now - 1, now, now + 1} {
		want, ok := totpAt(secret, c)
		if ok && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			k := secret + ":" + strconv.FormatUint(c, 10)
			if _, seen := usedCodes.LoadOrStore(k, time.Now()); seen {
				return false
			}
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- throttle

// Throttle slows guessing: at most limit failures per key per window.
type Throttle struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func NewThrottle(limit int, window time.Duration) *Throttle {
	return &Throttle{hits: map[string][]time.Time{}, limit: limit, window: window}
}

func (t *Throttle) prune(k string) []time.Time {
	cut := time.Now().Add(-t.window)
	h := t.hits[k]
	i := 0
	for i < len(h) && h[i].Before(cut) {
		i++
	}
	h = h[i:]
	if len(h) == 0 {
		delete(t.hits, k)
	} else {
		t.hits[k] = h
	}
	return h
}

// Blocked reports whether k has used up its failures.
func (t *Throttle) Blocked(k string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.prune(k)) >= t.limit
}

func (t *Throttle) Fail(k string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.hits[k] = append(t.prune(k), time.Now())
	if len(t.hits) > 10000 { // bound memory under a spray of addresses
		for kk := range t.hits {
			t.prune(kk)
		}
	}
}

func (t *Throttle) Clear(k string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.hits, k)
}
