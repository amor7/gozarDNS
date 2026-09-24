package hub

import (
	"errors"
	"io"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/catalog"
	"github.com/amor7/gozarDNS/internal/netx"
	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/store"
)

const userCookie = "gz_s"

// The customer API. Gates forward it from their portal; every request here
// has already passed gateAuth, so the client-address header is the gate's
// word, which is the only word that counts.

func (s *Hub) portalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /info", s.wrap(s.pInfo))
	mux.HandleFunc("POST /signup", s.wrap(s.pSignup))
	mux.HandleFunc("POST /login", s.wrap(s.pLogin))
	mux.HandleFunc("POST /logout", s.wrap(s.pLogout))
	mux.HandleFunc("GET /me", s.wrap(s.userOnly(s.pMe)))
	mux.HandleFunc("POST /devices", s.wrap(s.userOnly(s.pClaim)))
	mux.HandleFunc("PATCH /devices/{id}", s.wrap(s.userOnly(s.pRenameDevice)))
	mux.HandleFunc("DELETE /devices/{id}", s.wrap(s.userOnly(s.pDeleteDevice)))
	mux.HandleFunc("POST /password", s.wrap(s.userOnly(s.pPassword)))
	mux.HandleFunc("POST /payments", s.wrap(s.userOnly(s.pPayment)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireAJAX(r); err != nil {
			writeJSON(w, 403, err)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func clientAddr(r *http.Request) netip.Addr {
	a, _ := netip.ParseAddr(r.Header.Get(proto.HeaderClientIP))
	return a.Unmap()
}

type userHandler func(w http.ResponseWriter, r *http.Request, u *store.User) error

func (s *Hub) userOnly(h userHandler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		c, err := r.Cookie(userCookie)
		if err != nil {
			return errUnauthorized
		}
		ss, ok := s.store.Session("user", c.Value)
		if !ok {
			return errUnauthorized
		}
		u, err := s.store.User(ss.Subject)
		if err != nil {
			return errUnauthorized
		}
		return h(w, r, u)
	}
}

func setUserCookie(w http.ResponseWriter, tok string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: userCookie, Value: tok, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

// ---------------------------------------------------------------- public

type serviceBrief struct {
	ID    string       `json:"id"`
	Name  catalog.Text `json:"name"`
	Color string       `json:"color"`
	Heavy bool         `json:"heavy"` // includes a download group
}

// planServices lists the brands a plan carries, for display.
func (s *Hub) planServices(p *store.Plan) []serviceBrief {
	want := map[string]bool{}
	for _, g := range p.Groups {
		want[g] = true
	}
	out := []serviceBrief{}
	for _, sv := range s.cat.Services {
		var b *serviceBrief
		for _, g := range sv.Groups {
			if want[catalog.GroupKey(sv.ID, g.ID)] {
				if b == nil {
					b = &serviceBrief{ID: sv.ID, Name: sv.Name, Color: sv.Color}
				}
				b.Heavy = b.Heavy || g.Heavy
			}
		}
		if b != nil {
			out = append(out, *b)
		}
	}
	if want[CustomGroup] {
		out = append(out, serviceBrief{ID: "custom", Name: catalog.Text{FA: "دامنه‌های اختصاصی", EN: "Custom domains"}})
	}
	return out
}

type publicPlan struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Price        int64          `json:"price"`
	QuotaBytes   int64          `json:"quota_bytes"`
	DurationDays int            `json:"duration_days"`
	Renewal      string         `json:"renewal"`
	SpeedKbps    int            `json:"speed_kbps"`
	MaxDevices   int            `json:"max_devices"`
	Color        string         `json:"color"`
	Services     []serviceBrief `json:"services"`
}

func (s *Hub) publicPlan(p *store.Plan) publicPlan {
	return publicPlan{p.ID, p.Name, p.Description, p.Price, p.QuotaBytes, p.DurationDays, p.Renewal,
		p.SpeedKbps, p.MaxDevices, p.Color, s.planServices(p)}
}

func (s *Hub) dnsAddress(r *http.Request) string {
	if v := s.store.Setting(store.SetDNSAddress); v != "" {
		return v
	}
	return r.Header.Get(proto.HeaderGateIP)
}

func (s *Hub) pInfo(w http.ResponseWriter, r *http.Request) error {
	set := s.store.Settings()
	plans, err := s.store.Plans()
	if err != nil {
		return err
	}
	pub := []publicPlan{}
	for _, p := range plans {
		if p.Public {
			pub = append(pub, s.publicPlan(p))
		}
	}
	return ok(w, map[string]any{
		"brand":        set[store.SetBrand],
		"support":      set[store.SetSupport],
		"notice":       set[store.SetNotice],
		"currency":     set[store.SetCurrency],
		"payment_info": set[store.SetPaymentInfo],
		"signup_open":  set[store.SetSignupOpen] == "1",
		"online_pay":   set[store.SetIDPayKey] != "",
		"dns":          s.dnsAddress(r),
		"your_ip":      clientAddr(r).String(),
		"plans":        pub,
	})
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

// latinDigits turns Persian and Arabic-Indic digits into ASCII ones, so a
// phone number typed on a Persian keyboard is the same number.
func latinDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cleanPhone(s string) string {
	s = latinDigits(s)
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' || (r == '+' && b.Len() == 0) {
			b.WriteRune(r)
		}
	}
	return trimmed(b.String(), 20)
}

func (s *Hub) pSignup(w http.ResponseWriter, r *http.Request) error {
	if s.store.Setting(store.SetSignupOpen) != "1" {
		return fail(403, "signup_closed", "sign-up is closed")
	}
	ip := clientAddr(r).String()
	if s.signupThrottle.Blocked(ip) {
		return fail(429, "too_many_attempts", "too many sign-ups from this address")
	}
	var in struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Phone       string `json:"phone"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(latinDigits(in.Username)))
	if !usernameRe.MatchString(name) {
		return fail(400, "username_invalid", "3-32 letters, digits, dot, dash or underscore")
	}
	if len(in.Password) < 6 || len(in.Password) > 128 {
		return fail(400, "password_short", "at least 6 characters")
	}
	planID, _ := strconv.ParseInt(s.store.Setting(store.SetSignupPlan), 10, 64)
	if planID > 0 {
		if _, err := s.store.Plan(planID); err != nil {
			planID = 0
		}
	}
	u, err := s.store.CreateUser(store.NewUser{
		Username: name, DisplayName: trimmed(in.DisplayName, 60), Phone: cleanPhone(in.Phone),
		Password: auth.HashPassword(in.Password), PlanID: planID,
	})
	if errors.Is(err, store.ErrTaken) {
		return fail(409, "username_taken", "that username is taken")
	}
	if err != nil {
		return err
	}
	s.signupThrottle.Fail(ip) // counts sign-ups, not failures
	s.store.Log(u.Username, "signup", "signup", ip)

	// Register the address they signed up from, if it is free: most people
	// sign up on the network they want to use.
	if a := clientAddr(r); a.Is4() {
		if _, err := s.store.ClaimIP(u.ID, a.String(), "", 0); err == nil {
			s.store.Log(u.Username, "device", "device_added", a.String())
		}
	}
	tok, err := s.store.CreateSession("user", u.ID, 30*24*time.Hour, ip, r.UserAgent())
	if err != nil {
		return err
	}
	setUserCookie(w, tok, 30*86400)
	return ok(w, nil)
}

func (s *Hub) pLogin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	ip := clientAddr(r).String()
	name := strings.ToLower(strings.TrimSpace(latinDigits(in.Username)))
	if s.loginThrottle.Blocked("u:"+ip) || s.loginThrottle.Blocked("n:"+name) {
		return fail(429, "too_many_attempts", "too many attempts, wait a few minutes")
	}
	u, err := s.store.UserByName(name)
	if err != nil || u.PassHash() == "" {
		auth.BurnTime(in.Password)
		s.loginThrottle.Fail("u:" + ip)
		s.loginThrottle.Fail("n:" + name)
		return fail(401, "invalid_credentials", "wrong username or password")
	}
	if !auth.CheckPassword(u.PassHash(), in.Password) {
		s.loginThrottle.Fail("u:" + ip)
		s.loginThrottle.Fail("n:" + name)
		return fail(401, "invalid_credentials", "wrong username or password")
	}
	s.loginThrottle.Clear("n:" + name)
	tok, err := s.store.CreateSession("user", u.ID, 30*24*time.Hour, ip, r.UserAgent())
	if err != nil {
		return err
	}
	setUserCookie(w, tok, 30*86400)
	return ok(w, nil)
}

func (s *Hub) pLogout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(userCookie); err == nil {
		s.store.DeleteSession(c.Value)
	}
	setUserCookie(w, "", -1)
	return ok(w, nil)
}

// ---------------------------------------------------------------- account

func (s *Hub) pMe(w http.ResponseWriter, r *http.Request, u *store.User) error {
	devs, err := s.store.Devices(u.ID)
	if err != nil {
		return err
	}
	usage, err := s.store.DailyUsage(u.ID, 30)
	if err != nil {
		return err
	}
	pays, err := s.store.Payments("", u.ID, 10)
	if err != nil {
		return err
	}
	here := clientAddr(r)
	var thisDevice *int64
	ipTaken := false
	for _, d := range devs {
		if d.IP == here.String() {
			id := d.ID
			thisDevice = &id
		}
	}
	if thisDevice == nil {
		if d, err := s.store.DeviceByIP(here.String()); err == nil && d.UserID != u.ID {
			ipTaken = true
		}
	}
	var plan any
	if u.PlanID != nil {
		if p, err := s.store.Plan(*u.PlanID); err == nil {
			plan = s.publicPlan(p)
		}
	}
	return ok(w, map[string]any{
		"user": map[string]any{
			"id": u.ID, "username": u.Username, "display_name": u.DisplayName, "phone": u.Phone,
			"state": u.State, "quota_bytes": u.QuotaBytes, "used_bytes": u.UsedBytes,
			"period_start": u.PeriodStart, "expires_at": u.ExpiresAt, "speed_kbps": u.SpeedKbps,
			"max_devices": u.MaxDevices, "created_at": u.CreatedAt,
		},
		"plan":        plan,
		"devices":     devs,
		"current_ip":  here.String(),
		"ipv6":        here.Is6() && !here.Is4In6(),
		"this_device": thisDevice,
		"ip_taken":    ipTaken,
		"usage":       usage,
		"payments":    pays,
		"dns":         s.dnsAddress(r),
	})
}

func (s *Hub) pClaim(w http.ResponseWriter, r *http.Request, u *store.User) error {
	var in struct {
		Label   string `json:"label"`
		Replace int64  `json:"replace"`
		// IP registers an address other than the one this page is open on:
		// the home connection of a console, say, from a phone on mobile data.
		IP string `json:"ip"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	a := clientAddr(r)
	if in.IP != "" {
		m, err := netip.ParseAddr(strings.TrimSpace(latinDigits(in.IP)))
		if err != nil || !m.Is4() {
			return fail(400, "ip_invalid", "not an IPv4 address")
		}
		if !netx.IsPublic(m) {
			return fail(400, "ip_private", "that is an address inside a home network, not the internet address")
		}
		a = m
	} else if !a.Is4() {
		return fail(400, "ipv6_unsupported", "this connection is IPv6; open the page over IPv4 (turn Wi-Fi IPv6 off or use the address link)")
	}
	d, err := s.store.ClaimIP(u.ID, a.String(), trimmed(in.Label, 40), in.Replace)
	switch {
	case errors.Is(err, store.ErrAlreadyMine):
		return fail(409, "ip_already_yours", "this address is already registered to you")
	case errors.Is(err, store.ErrIPOwned):
		return fail(409, "ip_taken", "this address is registered to another account")
	case errors.Is(err, store.ErrTooMany):
		return fail(409, "device_limit", "device limit reached")
	case err != nil:
		return err
	}
	s.store.Log(u.Username, "device", "device_added", a.String())
	return ok(w, d)
}

func (s *Hub) pRenameDevice(w http.ResponseWriter, r *http.Request, u *store.User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Label string `json:"label"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if err := s.store.RenameDevice(u.ID, id, trimmed(in.Label, 40)); err != nil {
		return err
	}
	return ok(w, nil)
}

func (s *Hub) pDeleteDevice(w http.ResponseWriter, r *http.Request, u *store.User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.store.DeleteDevice(u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errNotFound
		}
		return err
	}
	s.store.Log(u.Username, "device", "device_removed")
	return ok(w, nil)
}

func (s *Hub) pPassword(w http.ResponseWriter, r *http.Request, u *store.User) error {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	if u.PassHash() != "" && !auth.CheckPassword(u.PassHash(), in.Current) {
		return fail(400, "password_wrong", "the current password is not right")
	}
	if len(in.New) < 6 || len(in.New) > 128 {
		return fail(400, "password_short", "at least 6 characters")
	}
	if err := s.store.SetUserPassword(u.ID, auth.HashPassword(in.New)); err != nil {
		return err
	}
	tok, err := s.store.CreateSession("user", u.ID, 30*24*time.Hour, clientAddr(r).String(), r.UserAgent())
	if err != nil {
		return err
	}
	setUserCookie(w, tok, 30*86400)
	return ok(w, nil)
}

const maxReceipt = 5 << 20

var receiptTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true, "application/pdf": true,
}

func (s *Hub) pPayment(w http.ResponseWriter, r *http.Request, u *store.User) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxReceipt+64<<10)
	if err := r.ParseMultipartForm(maxReceipt + 32<<10); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return fail(413, "receipt_too_large", "the file is larger than 5 MB")
		}
		return errBadRequest
	}
	f, _, err := r.FormFile("receipt")
	if err != nil {
		return fail(400, "receipt_missing", "attach a photo of the receipt")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxReceipt+1))
	if err != nil {
		return errBadRequest
	}
	if len(data) > maxReceipt {
		return fail(413, "receipt_too_large", "the file is larger than 5 MB")
	}
	ctype := http.DetectContentType(data)
	if !receiptTypes[ctype] {
		return fail(400, "receipt_type", "send a photo (JPG, PNG, WEBP) or a PDF")
	}
	amount, _ := strconv.ParseInt(strings.ReplaceAll(latinDigits(r.FormValue("amount")), ",", ""), 10, 64)
	var planID *int64
	if id, err := strconv.ParseInt(r.FormValue("plan_id"), 10, 64); err == nil && id > 0 {
		if p, err := s.store.Plan(id); err == nil {
			planID = &p.ID
			if amount <= 0 {
				amount = p.Price
			}
		}
	}
	id, err := s.store.CreatePayment(u.ID, planID, max(amount, 0), trimmed(r.FormValue("note"), 300), data, ctype)
	if errors.Is(err, store.ErrPendingExists) {
		return fail(409, "receipts_pending", "you already have receipts waiting for review")
	}
	if err != nil {
		return err
	}
	s.store.Log(u.Username, "payment", "payment_sent", strconv.FormatInt(id, 10))
	return ok(w, map[string]int64{"id": id})
}
