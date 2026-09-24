package hub

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/store"
)

// Online payment. The hub keeps the orders and the gateway credentials; the
// gate talks to the gateway, because Iranian gateways commonly refuse API
// calls from abroad. These endpoints sit behind gateAuth, never behind the
// customer-facing portal proxy, so credentials only ever travel hub -> gate.

// Gateways the hub knows, in the order customers see them.
var Gateways = []string{"zarinpal", "idpay", "payping"}

// gatewayCreds is the setting holding each gateway's credential, and the
// setting that holds its test-mode switch (if it has one).
var gatewayCreds = map[string][2]string{
	"idpay":    {store.SetIDPayKey, store.SetIDPaySandbox},
	"zarinpal": {store.SetZarinpalMerchant, store.SetZarinpalSandbox},
	"payping":  {store.SetPayPingToken, ""},
}

// enabledGateways lists gateways that are switched on and have a credential.
func (s *Hub) enabledGateways(set map[string]string) []string {
	out := []string{}
	for _, g := range Gateways {
		if set[gatewayCreds[g][0]] != "" && set["pay_"+g+"_on"] == "1" {
			out = append(out, g)
		}
	}
	return out
}

func orderID(id int64) string { return "gz-" + strconv.FormatInt(id, 10) }

func parseOrder(s string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(s, "gz-"), 10, 64)
	return id, err == nil && strings.HasPrefix(s, "gz-")
}

// rial converts a plan price into rial, which every gateway is told.
func (s *Hub) rial(amount int64) int64 {
	if s.store.Setting(store.SetPriceUnit) == "rial" {
		return amount
	}
	return amount * 10
}

func (s *Hub) order(o *store.OnlineOrder, set map[string]string) proto.PayOrder {
	c := gatewayCreds[o.Method]
	return proto.PayOrder{
		OrderID: orderID(o.ID), Gateway: o.Method, AmountRial: s.rial(o.Amount), Ref: o.Ref, Status: o.Status,
		Key: set[c[0]], Sandbox: c[1] != "" && set[c[1]] == "1",
	}
}

func (s *Hub) payStart(w http.ResponseWriter, r *http.Request, u *store.User) error {
	var in struct {
		PlanID  int64  `json:"plan_id"`
		Gateway string `json:"gateway"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	set := s.store.Settings()
	if !slices.Contains(s.enabledGateways(set), in.Gateway) {
		return fail(404, "online_pay_off", "that payment method is not available")
	}
	p, err := s.store.Plan(in.PlanID)
	if err != nil || !p.Public || p.Price <= 0 {
		return errNotFound
	}
	id, err := s.store.StartOnlinePayment(u.ID, p.ID, p.Price, in.Gateway)
	if err != nil {
		return err
	}
	o, err := s.store.OnlineOrder(id)
	if err != nil {
		return err
	}
	out := s.order(o, set)
	out.Name = u.DisplayName
	if out.Name == "" {
		out.Name = u.Username
	}
	out.Phone = u.Phone
	out.Desc = set[store.SetBrand] + " - " + p.Name
	return ok(w, out)
}

func (s *Hub) payRef(w http.ResponseWriter, r *http.Request) error {
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

func (s *Hub) payLookup(w http.ResponseWriter, r *http.Request) error {
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
	return ok(w, s.order(o, s.store.Settings()))
}

func (s *Hub) payFinish(w http.ResponseWriter, r *http.Request) error {
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
		name := ""
		if u, _ := s.store.User(o.UserID); u != nil {
			name = u.Username
		}
		s.store.Log(name, "payment", "payment_online", strconv.FormatInt(id, 10), in.TrackID)
	}
	return ok(w, map[string]bool{"fresh": fresh})
}
