package hub

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/store"
)

// Online payment through IDPay. The hub keeps the orders; the gate talks to
// IDPay, because Iranian gateways commonly refuse API calls from abroad. These
// endpoints sit behind gateAuth, never behind the customer-facing portal
// proxy, so the API key only ever travels hub -> gate.

func orderID(id int64) string { return "gz-" + strconv.FormatInt(id, 10) }

func parseOrder(s string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(s, "gz-"), 10, 64)
	return id, err == nil && strings.HasPrefix(s, "gz-")
}

// rial converts a plan price into what IDPay charges in.
func (s *Hub) rial(amount int64) int64 {
	if s.store.Setting(store.SetPriceUnit) == "rial" {
		return amount
	}
	return amount * 10
}

func (s *Hub) idpayStart(w http.ResponseWriter, r *http.Request, u *store.User) error {
	key := s.store.Setting(store.SetIDPayKey)
	if key == "" {
		return fail(404, "online_pay_off", "online payment is not set up")
	}
	var in struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	p, err := s.store.Plan(in.PlanID)
	if err != nil || !p.Public || p.Price <= 0 {
		return errNotFound
	}
	id, err := s.store.StartOnlinePayment(u.ID, p.ID, p.Price)
	if err != nil {
		return err
	}
	name := u.DisplayName
	if name == "" {
		name = u.Username
	}
	return ok(w, proto.IDPayOrder{
		OrderID: orderID(id), AmountRial: s.rial(p.Price), Name: name, Phone: u.Phone,
		Desc: s.store.Setting(store.SetBrand) + " - " + p.Name, APIKey: key,
		Sandbox: s.store.Setting(store.SetIDPaySandbox) == "1",
	})
}

func (s *Hub) idpayRef(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		OrderID string `json:"order_id"`
		Ref     string `json:"ref"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	id, good := parseOrder(in.OrderID)
	if !good {
		return errNotFound
	}
	if err := s.store.SetOnlineRef(id, trimmed(in.Ref, 100)); err != nil {
		return err
	}
	return ok(w, nil)
}

func (s *Hub) idpayLookup(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		OrderID string `json:"order_id"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	id, good := parseOrder(in.OrderID)
	if !good {
		return errNotFound
	}
	o, err := s.store.OnlineOrder(id)
	if err != nil {
		return errNotFound
	}
	return ok(w, proto.IDPayOrder{
		OrderID: orderID(o.ID), AmountRial: s.rial(o.Amount), Ref: o.Ref, Status: o.Status,
		APIKey: s.store.Setting(store.SetIDPayKey), Sandbox: s.store.Setting(store.SetIDPaySandbox) == "1",
	})
}

func (s *Hub) idpayFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		OrderID string `json:"order_id"`
		OK      bool   `json:"ok"`
		TrackID string `json:"track_id"`
		Note    string `json:"note"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	id, good := parseOrder(in.OrderID)
	if !good {
		return errNotFound
	}
	o, err := s.store.OnlineOrder(id)
	if err != nil {
		return errNotFound
	}
	fresh, err := s.store.FinishOnline(id, in.OK, trimmed(in.TrackID, 64), trimmed(in.Note, 200))
	if err != nil {
		return err
	}
	if fresh && in.OK && o.PlanID != nil {
		if err := s.store.ExtendPlan(o.UserID, *o.PlanID); err != nil {
			return err
		}
		u, _ := s.store.User(o.UserID)
		name := ""
		if u != nil {
			name = u.Username
		}
		s.store.Log(name, "payment", "payment_online", strconv.FormatInt(id, 10), in.TrackID)
	}
	return ok(w, map[string]bool{"fresh": fresh})
}
