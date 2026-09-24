package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h := HashPassword("correct horse")
	if !CheckPassword(h, "correct horse") {
		t.Fatal("rejected the right password")
	}
	if CheckPassword(h, "correct horsE") {
		t.Fatal("accepted a wrong password")
	}
	if CheckPassword("garbage", "x") {
		t.Fatal("accepted a malformed hash")
	}
}

func TestTOTP(t *testing.T) {
	s := NewTOTPSecret()
	code, _ := totpAt(s, uint64(time.Now().Unix()/30))
	if !CheckTOTP(s, code) {
		t.Fatal("current code rejected")
	}
	if CheckTOTP(s, code) {
		t.Fatal("replayed code accepted")
	}
	if CheckTOTP(s, "000000") && code != "000000" {
		t.Fatal("wrong code accepted")
	}
}

// RFC 6238 test vector for SHA-1 at T=59.
func TestTOTPVector(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // "12345678901234567890"
	got, _ := totpAt(secret, 59/30)
	if got != "287082" {
		t.Fatalf("got %s", got)
	}
}

func TestThrottle(t *testing.T) {
	th := NewThrottle(3, time.Minute)
	for i := 0; i < 3; i++ {
		if th.Blocked("a") {
			t.Fatal("blocked too early")
		}
		th.Fail("a")
	}
	if !th.Blocked("a") || th.Blocked("b") {
		t.Fatal("throttle wrong")
	}
}
