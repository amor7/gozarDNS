package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/amor7/gozarDNS/internal/proto"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "gozar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPlanLifecycleAndState(t *testing.T) {
	s := open(t)
	p := &Plan{Name: "Gamer", QuotaBytes: 100, DurationDays: 30, MaxDevices: 1, Groups: []string{"playstation/online"}}
	if err := s.SavePlan(p); err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(NewUser{Username: "sara"})
	if err != nil {
		t.Fatal(err)
	}
	if u.State != StateNoPlan {
		t.Fatalf("new account without plan: %s", u.State)
	}
	if err := s.ApplyPlan(u.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	u, _ = s.User(u.ID)
	if u.State != StateActive || u.QuotaBytes != 100 || u.MaxDevices != 1 || u.ExpiresAt == nil {
		t.Fatalf("after plan: %+v", u)
	}
	if _, err := s.CreateUser(NewUser{Username: "SARA"}); err != ErrTaken {
		t.Fatalf("usernames must be unique ignoring case, got %v", err)
	}
	if err := s.DeletePlan(p.ID); err != ErrInUse {
		t.Fatalf("deleting a plan in use: %v", err)
	}
}

func TestClaimIPRules(t *testing.T) {
	s := open(t)
	p := &Plan{Name: "One", MaxDevices: 1}
	_ = s.SavePlan(p)
	a, _ := s.CreateUser(NewUser{Username: "a", PlanID: p.ID})
	b, _ := s.CreateUser(NewUser{Username: "b", PlanID: p.ID})

	d1, err := s.ClaimIP(a.ID, "5.1.1.1", "phone", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimIP(a.ID, "5.1.1.1", "", 0); err != ErrAlreadyMine {
		t.Fatalf("re-claim: %v", err)
	}
	if _, err := s.ClaimIP(b.ID, "5.1.1.1", "", 0); err != ErrIPOwned {
		t.Fatalf("stealing: %v", err)
	}
	if _, err := s.ClaimIP(a.ID, "5.2.2.2", "", 0); err != ErrTooMany {
		t.Fatalf("over limit: %v", err)
	}
	if _, err := s.ClaimIP(a.ID, "5.2.2.2", "new", d1.ID); err != nil {
		t.Fatalf("replace: %v", err)
	}
	devs, _ := s.Devices(a.ID)
	if len(devs) != 1 || devs[0].IP != "5.2.2.2" {
		t.Fatalf("devices: %+v", devs)
	}
}

func TestSyncBatchIsCountedOnce(t *testing.T) {
	s := open(t)
	p := &Plan{Name: "Small", QuotaBytes: 1000, MaxDevices: 1}
	_ = s.SavePlan(p)
	u, _ := s.CreateUser(NewUser{Username: "ali", PlanID: p.ID})
	g, _, err := s.CreateGate("tehran")
	if err != nil {
		t.Fatal(err)
	}
	req := &proto.SyncRequest{Batch: "b1", Usage: map[int64]proto.Usage{u.ID: {Up: 200, Down: 900}}}
	for i := 0; i < 3; i++ {
		if _, err := s.RecordSync(g, "5.5.5.5", req); err != nil {
			t.Fatal(err)
		}
	}
	u, _ = s.User(u.ID)
	if u.UsedBytes != 1100 {
		t.Fatalf("used %d, want 1100", u.UsedBytes)
	}
	if u.State != StateOverQuota {
		t.Fatalf("state %s", u.State)
	}
	days, _ := s.DailyUsage(u.ID, 7)
	if len(days) != 7 || days[6].Bytes != 1100 {
		t.Fatalf("daily: %+v", days)
	}
	if ips := s.GateIPs(); len(ips) != 1 || ips[0] != "5.5.5.5" {
		t.Fatalf("gate ips %v", ips)
	}
}

func TestResetPlanRollsOver(t *testing.T) {
	s := open(t)
	p := &Plan{Name: "Monthly", QuotaBytes: 10, DurationDays: 30, Renewal: "reset", MaxDevices: 1}
	_ = s.SavePlan(p)
	u, _ := s.CreateUser(NewUser{Username: "m", PlanID: p.ID})
	past := time.Now().Add(-time.Hour).Unix()
	used := int64(10)
	_ = s.UpdateUser(u.ID, UserPatch{ExpiresAt: &past, UsedBytes: &used})
	if n, _ := s.RollPeriods(); n != 1 {
		t.Fatalf("rolled %d", n)
	}
	u, _ = s.User(u.ID)
	if u.UsedBytes != 0 || u.State != StateActive || *u.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("after roll: %+v", u)
	}
}

func TestBackupRoundTrip(t *testing.T) {
	s := open(t)
	_, _ = s.CreateUser(NewUser{Username: "keep-me"})
	dst := filepath.Join(t.TempDir(), "backup.db")
	if err := s.Backup(context.Background(), dst); err != nil {
		t.Fatal(err)
	}
	if err := CheckBackup(dst); err != nil {
		t.Fatal(err)
	}
	if err := s.StageRestore(dst); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := s2.UserByName("keep-me"); err != nil {
		t.Fatal(err)
	}
}
