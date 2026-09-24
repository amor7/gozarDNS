package hub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/buildinfo"
	"github.com/amor7/gozarDNS/internal/catalog"
	"github.com/amor7/gozarDNS/internal/rules"
	"github.com/amor7/gozarDNS/internal/store"
	"github.com/amor7/gozarDNS/internal/web"
)

const adminCookie = "gz_a"

type adminHandler func(w http.ResponseWriter, r *http.Request, a *store.Admin) error

func (s *Hub) adminHandler() http.Handler {
	mux := http.NewServeMux()
	A := func(pattern string, h adminHandler) { mux.HandleFunc(pattern, s.wrap(s.adminOnly(h))) }

	mux.HandleFunc("POST /api/login", s.wrap(s.aLogin))
	mux.HandleFunc("POST /api/logout", s.wrap(s.aLogout))
	A("GET /api/me", s.aMe)
	A("POST /api/me/password", s.aMyPassword)
	A("POST /api/me/totp/setup", s.aTOTPSetup)
	A("POST /api/me/totp/enable", s.aTOTPEnable)
	A("POST /api/me/totp/disable", s.aTOTPDisable)

	A("GET /api/overview", s.aOverview)
	A("GET /api/metrics", s.aMetrics)
	A("GET /api/activity", s.aActivity)

	A("GET /api/users", s.aUsers)
	A("POST /api/users", s.aCreateUser)
	A("GET /api/users/{id}", s.aUser)
	A("PATCH /api/users/{id}", s.aUpdateUser)
	A("DELETE /api/users/{id}", s.aDeleteUser)
	A("POST /api/users/{id}/plan", s.aUserPlan)
	A("POST /api/users/{id}/reset-usage", s.aResetUsage)
	A("POST /api/users/{id}/password", s.aUserPassword)
	A("POST /api/users/{id}/devices", s.aAddDevice)
	A("DELETE /api/users/{id}/devices/{did}", s.aDeleteDevice)

	A("GET /api/plans", s.aPlans)
	A("POST /api/plans", s.aSavePlan)
	A("PUT /api/plans/{id}", s.aSavePlan)
	A("DELETE /api/plans/{id}", s.aDeletePlan)

	A("GET /api/catalog", s.aCatalog)
	A("POST /api/domains", s.aAddDomain)
	A("DELETE /api/domains/{domain}", s.aDeleteDomain)
	A("PUT /api/rules/{id}", s.aToggleRule)
	A("GET /api/lookup", s.aLookup)

	A("GET /api/payments", s.aPayments)
	A("GET /api/payments/{id}/receipt", s.aReceipt)
	A("POST /api/payments/{id}/approve", s.aApprove)
	A("POST /api/payments/{id}/reject", s.aReject)

	A("GET /api/gates", s.aGates)
	A("POST /api/gates", s.aCreateGate)
	A("PATCH /api/gates/{id}", s.aUpdateGate)
	A("POST /api/gates/{id}/token", s.aGateToken)
	A("DELETE /api/gates/{id}", s.aDeleteGate)

	A("GET /api/settings", s.aSettings)
	A("PATCH /api/settings", s.aUpdateSettings)
	A("GET /api/admins", s.aAdmins)
	A("POST /api/admins", s.aCreateAdmin)
	A("DELETE /api/admins/{id}", s.aDeleteAdmin)
	A("GET /api/backup", s.aBackup)
	A("POST /api/restore", s.aRestore)

	mux.Handle("/api/", s.wrap(func(http.ResponseWriter, *http.Request) error { return errNotFound }))
	mux.Handle("/", web.App("admin"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if err := requireAJAX(r); err != nil {
				writeJSON(w, 403, err)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Hub) adminOnly(h adminHandler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		c, err := r.Cookie(adminCookie)
		if err != nil {
			return errUnauthorized
		}
		ss, ok := s.store.Session("admin", c.Value)
		if !ok {
			return errUnauthorized
		}
		a, err := s.store.Admin(ss.Subject)
		if err != nil {
			return errUnauthorized
		}
		return h(w, r, a)
	}
}

// ---------------------------------------------------------------- auth

func (s *Hub) aLogin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	ip := remoteIP(r)
	if s.loginThrottle.Blocked("a:"+ip) || s.loginThrottle.Blocked("an:"+strings.ToLower(in.Username)) {
		return fail(429, "too_many_attempts", "too many attempts, wait a few minutes")
	}
	bad := func() error {
		s.loginThrottle.Fail("a:" + ip)
		s.loginThrottle.Fail("an:" + strings.ToLower(in.Username))
		s.log.Warn("admin sign-in failed", "ip", ip, "user", in.Username)
		return fail(401, "invalid_credentials", "wrong username or password")
	}
	a, err := s.store.AdminByName(in.Username)
	if err != nil {
		auth.BurnTime(in.Password)
		return bad()
	}
	if !auth.CheckPassword(a.PassHash, in.Password) {
		return bad()
	}
	if a.TOTPEnabled {
		if strings.TrimSpace(in.Code) == "" {
			return fail(401, "totp_required", "enter the code from your authenticator app")
		}
		if !auth.CheckTOTP(a.TOTPSecret, latinDigits(in.Code)) {
			return bad()
		}
	}
	tok, err := s.store.CreateSession("admin", a.ID, 7*24*time.Hour, ip, r.UserAgent())
	if err != nil {
		return err
	}
	s.store.TouchAdminLogin(a.ID)
	s.store.Log(a.Username, "login", "login", ip)
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: tok, Path: "/", MaxAge: 7 * 86400,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	return ok(w, nil)
}

func (s *Hub) aLogout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(adminCookie); err == nil {
		s.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	return ok(w, nil)
}

func (s *Hub) aMe(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	return ok(w, map[string]any{"admin": a, "brand": s.store.Setting(store.SetBrand), "version": buildinfo.Version,
		"pending_payments": s.store.PendingPayments()})
}

func (s *Hub) aMyPassword(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in struct{ Current, New string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if !auth.CheckPassword(a.PassHash, in.Current) {
		return fail(400, "password_wrong", "the current password is not right")
	}
	if len(in.New) < 10 {
		return fail(400, "password_short", "at least 10 characters for an admin")
	}
	if err := s.store.SetAdminPassword(a.ID, auth.HashPassword(in.New)); err != nil {
		return err
	}
	s.store.Log(a.Username, "security", "own_password")
	return ok(w, nil)
}

func (s *Hub) aTOTPSetup(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	if a.TOTPEnabled {
		return fail(409, "totp_enabled", "two-step sign-in is already on")
	}
	secret := auth.NewTOTPSecret()
	if err := s.store.SetAdminTOTP(a.ID, secret, false); err != nil {
		return err
	}
	return ok(w, map[string]string{"secret": secret, "uri": auth.TOTPURI(secret, s.store.Setting(store.SetBrand), a.Username)})
}

func (s *Hub) aTOTPEnable(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in struct{ Code string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if a.TOTPSecret == "" || !auth.CheckTOTP(a.TOTPSecret, latinDigits(in.Code)) {
		return fail(400, "totp_invalid", "that code is not right - check the time on your phone")
	}
	if err := s.store.SetAdminTOTP(a.ID, a.TOTPSecret, true); err != nil {
		return err
	}
	s.store.Log(a.Username, "security", "2fa_on")
	return ok(w, nil)
}

func (s *Hub) aTOTPDisable(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in struct{ Password string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if !auth.CheckPassword(a.PassHash, in.Password) {
		return fail(400, "password_wrong", "the password is not right")
	}
	if err := s.store.SetAdminTOTP(a.ID, "", false); err != nil {
		return err
	}
	s.store.Log(a.Username, "security", "2fa_off")
	return ok(w, nil)
}

// ---------------------------------------------------------------- dashboard

func (s *Hub) aOverview(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	traffic, err := s.store.Traffic(24*time.Hour, 30*time.Minute)
	if err != nil {
		return err
	}
	daily, err := s.store.DailyUsage(0, 30)
	if err != nil {
		return err
	}
	top, err := s.store.TopUsers(7, 6)
	if err != nil {
		return err
	}
	gates, err := s.store.Gates()
	if err != nil {
		return err
	}
	act, _ := s.store.Activity(8, 0)
	expiring, _, _ := s.store.Users(store.UserFilter{State: store.StateActive, Sort: "expiry", Limit: 6})
	soon := []*store.User{}
	for _, u := range expiring {
		if u.ExpiresAt != nil && *u.ExpiresAt-time.Now().Unix() < 5*86400 {
			soon = append(soon, u)
		}
	}
	hub, _ := s.store.Metrics("hub", 10*time.Minute, 10)
	var hubNow any
	if len(hub) > 0 {
		hubNow = hub[len(hub)-1]
	}
	return ok(w, map[string]any{
		"totals":   s.store.Totals(),
		"traffic":  traffic,
		"daily":    daily,
		"top":      top,
		"gates":    gates,
		"activity": act,
		"expiring": soon,
		"hub":      map[string]any{"now": hubNow, "sample": s.sampler.Take(), "ipv6": s.ipv6.Load(), "conns": s.openConns.Load()},
	})
}

func (s *Hub) aMetrics(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	node := r.URL.Query().Get("node")
	if node == "" {
		node = "hub"
	}
	span := 24 * time.Hour
	switch r.URL.Query().Get("range") {
	case "1h":
		span = time.Hour
	case "7d":
		span = 7 * 24 * time.Hour
	}
	pts, err := s.store.Metrics(node, span, 120)
	if err != nil {
		return err
	}
	return ok(w, pts)
}

func (s *Hub) aActivity(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	act, err := s.store.Activity(min(max(queryInt(r, "limit", 50), 1), 200), before)
	if err != nil {
		return err
	}
	return ok(w, act)
}

// ---------------------------------------------------------------- users

func (s *Hub) aUsers(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	q := r.URL.Query()
	planID, _ := strconv.ParseInt(q.Get("plan"), 10, 64)
	users, total, err := s.store.Users(store.UserFilter{
		Query: q.Get("q"), State: q.Get("state"), PlanID: planID, Sort: q.Get("sort"),
		Limit: min(max(queryInt(r, "limit", 50), 1), 500), Offset: max(queryInt(r, "offset", 0), 0),
	})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"users": users, "total": total})
}

func (s *Hub) aCreateUser(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Phone       string `json:"phone"`
		PlanID      int64  `json:"plan_id"`
		Note        string `json:"note"`
		IP          string `json:"ip"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(latinDigits(in.Username)))
	if !usernameRe.MatchString(name) {
		return fail(400, "username_invalid", "3-32 letters, digits, dot, dash or underscore")
	}
	generated := ""
	if in.Password == "" {
		generated = auth.Password(10)
		in.Password = generated
	}
	if len(in.Password) < 6 {
		return fail(400, "password_short", "at least 6 characters")
	}
	ip := strings.TrimSpace(latinDigits(in.IP))
	if ip != "" && net.ParseIP(ip).To4() == nil {
		return fail(400, "ip_invalid", "not an IPv4 address")
	}
	u, err := s.store.CreateUser(store.NewUser{Username: name, DisplayName: trimmed(in.DisplayName, 60),
		Phone: cleanPhone(in.Phone), Password: auth.HashPassword(in.Password), PlanID: in.PlanID, Note: trimmed(in.Note, 500)})
	if errors.Is(err, store.ErrTaken) {
		return fail(409, "username_taken", "that username is taken")
	}
	if err != nil {
		return err
	}
	if ip != "" {
		if err := s.store.AddDeviceAdmin(u.ID, ip, ""); err != nil {
			s.log.Warn("could not register address for new user", "ip", ip, "err", err)
		}
	}
	s.store.Log(a.Username, "user", "user_created", u.Username)
	return ok(w, map[string]any{"user": u, "password": generated})
}

func (s *Hub) aUser(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	u, err := s.store.User(id)
	if err != nil {
		return errNotFound
	}
	devs, _ := s.store.Devices(id)
	usage, _ := s.store.DailyUsage(id, 30)
	pays, _ := s.store.Payments("", id, 20)
	return ok(w, map[string]any{"user": u, "devices": devs, "usage": usage, "payments": pays})
}

func (s *Hub) aUpdateUser(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var p store.UserPatch
	if err := readJSON(r, &p); err != nil {
		return err
	}
	if err := s.store.UpdateUser(id, p); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errNotFound
		}
		return err
	}
	u, _ := s.store.User(id)
	s.store.Log(a.Username, "user", "user_edited", u.Username)
	return ok(w, u)
}

func (s *Hub) aDeleteUser(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	u, err := s.store.User(id)
	if err != nil {
		return errNotFound
	}
	if err := s.store.DeleteUser(id); err != nil {
		return err
	}
	s.store.Log(a.Username, "user", "user_deleted", u.Username)
	return ok(w, nil)
}

func (s *Hub) aUserPlan(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		PlanID int64  `json:"plan_id"`
		Mode   string `json:"mode"` // "apply" (start fresh) or "extend"
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if in.Mode == "extend" {
		err = s.store.ExtendPlan(id, in.PlanID)
	} else {
		err = s.store.ApplyPlan(id, in.PlanID)
	}
	if errors.Is(err, store.ErrNotFound) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	u, _ := s.store.User(id)
	s.store.Log(a.Username, "plan", map[bool]string{true: "plan_extended", false: "plan_started"}[in.Mode == "extend"], u.Username, u.PlanName)
	return ok(w, u)
}

func (s *Hub) aResetUsage(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.store.ResetUsage(id); err != nil {
		return err
	}
	u, _ := s.store.User(id)
	s.store.Log(a.Username, "user", "usage_reset", u.Username)
	return ok(w, u)
}

func (s *Hub) aUserPassword(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct{ Password string }
	_ = readJSON(r, &in)
	pw := in.Password
	if pw == "" {
		pw = auth.Password(10)
	}
	if len(pw) < 6 {
		return fail(400, "password_short", "at least 6 characters")
	}
	if err := s.store.SetUserPassword(id, auth.HashPassword(pw)); err != nil {
		return err
	}
	u, _ := s.store.User(id)
	s.store.Log(a.Username, "user", "user_password", u.Username)
	return ok(w, map[string]string{"password": pw})
}

func (s *Hub) aAddDevice(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct{ IP, Label string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	ip := strings.TrimSpace(latinDigits(in.IP))
	if net.ParseIP(ip).To4() == nil || strings.Contains(ip, ":") {
		return fail(400, "ip_invalid", "not an IPv4 address")
	}
	switch err := s.store.AddDeviceAdmin(id, ip, trimmed(in.Label, 40)); {
	case errors.Is(err, store.ErrIPOwned):
		return fail(409, "ip_taken", "that address is registered to another account")
	case errors.Is(err, store.ErrAlreadyMine):
		return fail(409, "ip_already_yours", "already registered to this account")
	case err != nil:
		return err
	}
	u, _ := s.store.User(id)
	s.store.Log(a.Username, "device", "device_added_for", ip, u.Username)
	return ok(w, nil)
}

func (s *Hub) aDeleteDevice(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	did, err := pathID(r, "did")
	if err != nil {
		return err
	}
	if err := s.store.DeleteDevice(id, did); err != nil {
		return errNotFound
	}
	u, _ := s.store.User(id)
	s.store.Log(a.Username, "device", "device_removed_for", u.Username)
	return ok(w, nil)
}

// ---------------------------------------------------------------- plans

func (s *Hub) aPlans(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	plans, err := s.store.Plans()
	if err != nil {
		return err
	}
	type withDomains struct {
		*store.Plan
		Domains int `json:"domains"`
	}
	custom, _ := s.store.CustomDomains()
	direct, pin := s.exceptions(custom)
	out := []withDomains{}
	for _, p := range plans {
		out = append(out, withDomains{p, len(s.planSpec(p, custom, direct, pin).Route)})
	}
	return ok(w, out)
}

func (s *Hub) aSavePlan(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var p store.Plan
	if err := readJSON(r, &p); err != nil {
		return err
	}
	if r.Method == http.MethodPut {
		id, err := pathID(r, "id")
		if err != nil {
			return err
		}
		p.ID = id
	} else {
		p.ID = 0
	}
	valid := map[string]bool{CustomGroup: true}
	for _, k := range s.cat.AllGroupKeys() {
		valid[k] = true
	}
	groups := []string{}
	for _, g := range p.Groups {
		if valid[g] && !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	p.Groups = groups
	excl := []string{}
	for _, d := range p.Excluded {
		if d = rules.Normalize(d); rules.ValidDomain(d) {
			excl = append(excl, d)
		}
	}
	p.Excluded = excl
	p.Name = trimmed(p.Name, 40)
	p.Description = trimmed(p.Description, 300)
	if p.Name == "" {
		return fail(400, "name_required", "a plan needs a name")
	}
	if err := s.store.SavePlan(&p); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errNotFound
		}
		return err
	}
	s.store.Log(a.Username, "plan", "plan_saved", p.Name)
	saved, _ := s.store.Plan(p.ID)
	return ok(w, saved)
}

func (s *Hub) aDeletePlan(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.store.Plan(id)
	if err != nil {
		return errNotFound
	}
	if err := s.store.DeletePlan(id); err != nil {
		if errors.Is(err, store.ErrInUse) {
			return fail(409, "plan_in_use", "move this plan's customers to another plan first")
		}
		return err
	}
	s.store.Log(a.Username, "plan", "plan_deleted", p.Name)
	return ok(w, nil)
}

// ---------------------------------------------------------------- domains

func (s *Hub) aCatalog(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	custom, err := s.store.CustomDomains()
	if err != nil {
		return err
	}
	var off []string
	_ = json.Unmarshal([]byte(s.store.Setting(store.SetDisabledRules)), &off)
	type ruleOut struct {
		catalog.Rule
		Enabled bool `json:"enabled"`
	}
	rs := []ruleOut{}
	for _, r := range s.cat.Rules {
		rs = append(rs, ruleOut{r, !slices.Contains(off, r.ID)})
	}
	return ok(w, map[string]any{
		"categories": s.cat.Categories,
		"services":   s.cat.Services,
		"rules":      rs,
		"custom":     custom,
		"custom_key": CustomGroup,
	})
}

func (s *Hub) aAddDomain(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in store.CustomDomain
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Domain = rules.Normalize(in.Domain)
	if !rules.ValidDomain(in.Domain) {
		return fail(400, "domain_invalid", "that is not a domain name")
	}
	in.Note = trimmed(in.Note, 200)
	if err := s.store.AddCustomDomain(in); err != nil {
		return err
	}
	s.store.Log(a.Username, "domain", "domain_added", in.Domain, in.Action)
	return ok(w, nil)
}

func (s *Hub) aDeleteDomain(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	d := rules.Normalize(r.PathValue("domain"))
	if err := s.store.DeleteCustomDomain(d); err != nil {
		return errNotFound
	}
	s.store.Log(a.Username, "domain", "domain_removed", d)
	return ok(w, nil)
}

func (s *Hub) aToggleRule(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id := r.PathValue("id")
	var in struct{ Enabled bool }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	known := false
	for _, rl := range s.cat.Rules {
		known = known || rl.ID == id
	}
	if !known {
		return errNotFound
	}
	var off []string
	_ = json.Unmarshal([]byte(s.store.Setting(store.SetDisabledRules)), &off)
	off = slices.DeleteFunc(off, func(x string) bool { return x == id })
	if !in.Enabled {
		off = append(off, id)
	}
	b, _ := json.Marshal(off)
	if err := s.store.SetSetting(store.SetDisabledRules, string(b)); err != nil {
		return err
	}
	s.store.Log(a.Username, "domain", map[bool]string{true: "rule_on", false: "rule_off"}[in.Enabled], id)
	return ok(w, nil)
}

// aLookup explains what happens to a name under every plan.
func (s *Hub) aLookup(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	name := rules.Normalize(r.URL.Query().Get("name"))
	if !rules.ValidDomain(name) {
		return fail(400, "domain_invalid", "that is not a domain name")
	}
	plans, err := s.store.Plans()
	if err != nil {
		return err
	}
	custom, _ := s.store.CustomDomains()
	direct, pin := s.exceptions(custom)
	type verdict struct {
		PlanID  int64  `json:"plan_id"`
		Plan    string `json:"plan"`
		Action  string `json:"action"`
		Matched string `json:"matched"`
	}
	out := []verdict{}
	for _, p := range plans {
		act, m := rules.Compile(s.planSpec(p, custom, direct, pin)).Match(name)
		out = append(out, verdict{p.ID, p.Name, act.String(), m})
	}
	// Which catalogue entry the name falls under, walking up its parents.
	var owner map[string]any
	for n := name; n != ""; {
		if sv, g := s.cat.Owner(n); sv != nil {
			owner = map[string]any{"service": sv.Name, "group": g.Name, "domain": n}
			break
		}
		i := strings.IndexByte(n, '.')
		if i < 0 {
			break
		}
		n = n[i+1:]
	}
	return ok(w, map[string]any{"name": name, "plans": out, "owner": owner})
}

// ---------------------------------------------------------------- payments

func (s *Hub) aPayments(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	p, err := s.store.Payments(r.URL.Query().Get("status"), 0, min(max(queryInt(r, "limit", 100), 1), 500))
	if err != nil {
		return err
	}
	return ok(w, p)
}

func (s *Hub) aReceipt(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	b, ctype, err := s.store.Receipt(id)
	if err != nil {
		return errNotFound
	}
	if !receiptTypes[ctype] {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
	return nil
}

func (s *Hub) aApprove(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Mode   string `json:"mode"` // "extend", "apply" or "none"
		PlanID int64  `json:"plan_id"`
		Note   string `json:"note"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	p, err := s.store.Payment(id)
	if err != nil {
		return errNotFound
	}
	if in.PlanID == 0 && p.PlanID != nil {
		in.PlanID = *p.PlanID
	}
	if err := s.store.DecidePayment(id, store.PayApproved, trimmed(in.Note, 300), a.Username); err != nil {
		return fail(409, "already_decided", "this receipt was already reviewed")
	}
	switch {
	case in.Mode == "none" || in.PlanID == 0:
	case in.Mode == "apply":
		err = s.store.ApplyPlan(p.UserID, in.PlanID)
	default:
		err = s.store.ExtendPlan(p.UserID, in.PlanID)
	}
	if err != nil {
		return err
	}
	s.store.Log(a.Username, "payment", "payment_approved", strconv.FormatInt(id, 10), p.Username)
	return ok(w, nil)
}

func (s *Hub) aReject(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct{ Note string }
	_ = readJSON(r, &in)
	p, err := s.store.Payment(id)
	if err != nil {
		return errNotFound
	}
	if err := s.store.DecidePayment(id, store.PayRejected, trimmed(in.Note, 300), a.Username); err != nil {
		return fail(409, "already_decided", "this receipt was already reviewed")
	}
	s.store.Log(a.Username, "payment", "payment_rejected", strconv.FormatInt(id, 10), p.Username)
	return ok(w, nil)
}

// ---------------------------------------------------------------- gates

func (s *Hub) aGates(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	g, err := s.store.Gates()
	if err != nil {
		return err
	}
	return ok(w, g)
}

func (s *Hub) aCreateGate(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in struct{ Name string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	name := trimmed(in.Name, 40)
	if name == "" {
		name = "gate"
	}
	g, secret, err := s.store.CreateGate(name)
	if err != nil {
		return err
	}
	s.store.Log(a.Username, "gate", "gate_added", name)
	return ok(w, map[string]any{"gate": g, "token": JoinToken(s.info(), g.ID, secret)})
}

func (s *Hub) aGateToken(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id := r.PathValue("id")
	secret, err := s.store.RotateGateSecret(id)
	if err != nil {
		return errNotFound
	}
	s.store.Log(a.Username, "gate", "gate_token", id)
	return ok(w, map[string]string{"token": JoinToken(s.info(), id, secret)})
}

func (s *Hub) aUpdateGate(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in struct {
		Name    *string `json:"name"`
		Enforce *string `json:"enforce"`
		Tunnel  *string `json:"tunnel"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	id := r.PathValue("id")
	if _, err := s.store.Gate(id); err != nil {
		return errNotFound
	}
	if err := s.store.UpdateGate(id, in.Name, in.Enforce, in.Tunnel); err != nil {
		return fail(400, "bad_request", err.Error())
	}
	s.store.Log(a.Username, "gate", "gate_updated", id)
	return ok(w, nil)
}

func (s *Hub) aDeleteGate(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id := r.PathValue("id")
	if err := s.store.DeleteGate(id); err != nil {
		return errNotFound
	}
	s.refreshAllowed()
	s.store.Log(a.Username, "gate", "gate_removed", id)
	return ok(w, nil)
}

// ---------------------------------------------------------------- settings

var editableSettings = []string{store.SetBrand, store.SetSupport, store.SetPaymentInfo, store.SetSignupOpen,
	store.SetSignupPlan, store.SetNotice, store.SetCurrency, store.SetDNSAddress, store.SetIDPaySandbox, store.SetPriceUnit,
	store.SetCardPay, store.SetZarinpalSandbox, "pay_idpay_on", "pay_zarinpal_on", "pay_payping_on"}

// secretSettings are write-only: the console can set or clear them, and learns
// only whether one is set.
var secretSettings = []string{store.SetIDPayKey, store.SetZarinpalMerchant, store.SetPayPingToken}

func (s *Hub) aSettings(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	all := s.store.Settings()
	out := map[string]string{}
	for _, k := range editableSettings {
		out[k] = all[k]
	}
	// The key is write-only: the console learns whether one is set, never what.
	for _, k := range secretSettings {
		if all[k] != "" {
			out[k+"_set"] = "1"
		}
	}
	issued, until := s.adminCert.Status()
	var certUntil int64
	if issued {
		certUntil = until.Unix()
	}
	return ok(w, map[string]any{
		"settings": out,
		"system": map[string]any{
			"version": buildinfo.Version, "public_ip": s.h.PublicIP, "admin_url": s.AdminURL(),
			"admin_domain": s.h.AdminDomain, "cert_until": certUntil, "ipv6_egress": s.ipv6.Load(),
			"control_port": s.h.ControlPort, "fingerprint": s.ControlFingerprint(),
		},
	})
}

func (s *Hub) aUpdateSettings(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in map[string]string
	if err := readJSON(r, &in); err != nil {
		return err
	}
	for _, k := range secretSettings {
		v, set := in[k]
		if !set {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) > 300 || strings.IndexFunc(v, unicode.IsSpace) >= 0 {
			return fail(400, "bad_request", "that does not look like a gateway key")
		}
		if err := s.store.SetSetting(k, v); err != nil {
			return err
		}
	}
	for k, v := range in {
		if !slices.Contains(editableSettings, k) {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case store.SetSignupOpen, store.SetIDPaySandbox, store.SetZarinpalSandbox, store.SetCardPay,
			"pay_idpay_on", "pay_zarinpal_on", "pay_payping_on":
			if v != "1" {
				v = "0"
			}
		case store.SetSignupPlan:
			if _, err := strconv.ParseInt(v, 10, 64); err != nil {
				v = "0"
			}
		case store.SetPriceUnit:
			if v != "rial" {
				v = "toman"
			}
		case store.SetDNSAddress:
			if v != "" && net.ParseIP(v) == nil {
				return fail(400, "ip_invalid", "the DNS address must be an IP address")
			}
		default:
			v = trimmed(v, 2000)
		}
		if err := s.store.SetSetting(k, v); err != nil {
			return err
		}
	}
	s.store.Log(a.Username, "settings", "settings_changed")
	return s.aSettings(w, r, a)
}

func (s *Hub) aAdmins(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	list, err := s.store.Admins()
	if err != nil {
		return err
	}
	return ok(w, list)
}

func (s *Hub) aCreateAdmin(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	var in struct{ Username, Password string }
	if err := readJSON(r, &in); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	if !usernameRe.MatchString(name) {
		return fail(400, "username_invalid", "3-32 letters, digits, dot, dash or underscore")
	}
	if len(in.Password) < 10 {
		return fail(400, "password_short", "at least 10 characters for an admin")
	}
	na, err := s.store.CreateAdmin(name, auth.HashPassword(in.Password))
	if errors.Is(err, store.ErrTaken) {
		return fail(409, "username_taken", "that username is taken")
	}
	if err != nil {
		return err
	}
	s.store.Log(a.Username, "security", "admin_added", na.Username)
	return ok(w, na)
}

func (s *Hub) aDeleteAdmin(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if id == a.ID {
		return fail(400, "cannot_delete_self", "you cannot remove yourself")
	}
	if err := s.store.DeleteAdmin(id); err != nil {
		return fail(400, "last_admin", err.Error())
	}
	s.store.Log(a.Username, "security", "admin_removed")
	return ok(w, nil)
}

// ---------------------------------------------------------------- backup

func (s *Hub) aBackup(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	tmp := filepath.Join(s.cfg.DataDir, "backup-download.db")
	defer os.Remove(tmp)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.store.Backup(ctx, tmp); err != nil {
		return err
	}
	f, err := os.Open(tmp)
	if err != nil {
		return err
	}
	defer f.Close()
	name := "gozar-backup-" + time.Now().Format("2006-01-02-1504") + ".db"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	s.store.Log(a.Username, "backup", "backup_downloaded")
	_, err = io.Copy(w, f)
	return err
}

func (s *Hub) aRestore(w http.ResponseWriter, r *http.Request, a *store.Admin) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
	f, _, err := r.FormFile("backup")
	if err != nil {
		return fail(400, "backup_missing", "choose a backup file")
	}
	defer f.Close()
	tmp := filepath.Join(s.cfg.DataDir, "restore-upload.db")
	defer os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		return err
	}
	out.Close()
	// Keep a copy of what is being replaced, whatever happens next.
	_ = s.store.Backup(r.Context(), filepath.Join(s.cfg.DataDir, "before-restore-"+time.Now().Format("20060102-150405")+".db"))
	if err := s.store.StageRestore(tmp); err != nil {
		return fail(400, "backup_invalid", err.Error())
	}
	s.store.Log(a.Username, "backup", "backup_restored")
	_ = ok(w, nil)
	go func() {
		time.Sleep(time.Second)
		close(s.restart)
	}()
	return nil
}
