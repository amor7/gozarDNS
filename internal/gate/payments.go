package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/web"
)

// Gateway endpoints. Variables so tests can point them at a fake.
var (
	IDPayBase       = "https://api.idpay.ir/v1.1"
	ZarinpalBase    = "https://payment.zarinpal.com"
	ZarinpalSandbox = "https://sandbox.zarinpal.com"
	PayPingBase     = "https://api.payping.ir/v2"

	// PayClient calls the gateways.
	PayClient = &http.Client{Timeout: 20 * time.Second}
)

// A gateway opens a payment and, when the customer comes back, confirms with
// the gateway itself that the money arrived. Nothing the browser carries back
// is believed until verify says so.
type gateway interface {
	// create returns the gateway's id for the payment and where to send the customer.
	create(ctx context.Context, o *proto.PayOrder, callback string) (ref, link string, err error)
	// verify checks the callback against the gateway. ok=false with a nil
	// error means the customer cancelled or the bank refused.
	verify(ctx context.Context, o *proto.PayOrder, f url.Values) (ok bool, track string, err error)
}

var gateways = map[string]gateway{"idpay": idpay{}, "zarinpal": zarinpal{}, "payping": payping{}}

var errForged = errors.New("callback does not match the payment")

// postJSON sends in to url and decodes a 2xx answer into out. Non-2xx
// answers come back as errors carrying the body, which is where every one of
// these gateways explains itself.
func postJSON(ctx context.Context, url string, headers map[string]string, in, out any) error {
	b, _ := json.Marshal(in)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := PayClient.Do(req)
	if err != nil {
		return fmt.Errorf("gateway unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway answered %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

var mobileRe = regexp.MustCompile(`^09\d{9}$`)

// ---------------------------------------------------------------- IDPay

type idpay struct{}

func (idpay) headers(o *proto.PayOrder) map[string]string {
	h := map[string]string{"X-API-KEY": o.Key}
	if o.Sandbox {
		h["X-SANDBOX"] = "1"
	}
	return h
}

func (g idpay) create(ctx context.Context, o *proto.PayOrder, callback string) (string, string, error) {
	var r struct {
		ID   string `json:"id"`
		Link string `json:"link"`
	}
	err := postJSON(ctx, IDPayBase+"/payment", g.headers(o), map[string]any{
		"order_id": o.OrderID, "amount": o.AmountRial, "name": o.Name, "phone": o.Phone,
		"desc": o.Desc, "callback": callback,
	}, &r)
	if err == nil && (r.ID == "" || r.Link == "") {
		err = errors.New("idpay returned no payment link")
	}
	return r.ID, r.Link, err
}

func (g idpay) verify(ctx context.Context, o *proto.PayOrder, f url.Values) (bool, string, error) {
	if f.Get("id") != o.Ref || f.Get("order_id") != o.OrderID {
		return false, "", errForged
	}
	if f.Get("status") != "10" { // 10: paid, waiting to be verified
		return false, f.Get("track_id"), nil
	}
	var v struct {
		Status  int         `json:"status"`
		TrackID json.Number `json:"track_id"`
		Amount  json.Number `json:"amount"`
		OrderID string      `json:"order_id"`
	}
	if err := postJSON(ctx, IDPayBase+"/payment/verify", g.headers(o), map[string]string{"id": o.Ref, "order_id": o.OrderID}, &v); err != nil {
		return false, "", err
	}
	amount, _ := v.Amount.Int64()
	good := (v.Status == 100 || v.Status == 101) && v.OrderID == o.OrderID && amount == o.AmountRial
	return good, v.TrackID.String(), nil
}

// ---------------------------------------------------------------- ZarinPal

type zarinpal struct{}

func (zarinpal) base(o *proto.PayOrder) string {
	if o.Sandbox {
		return ZarinpalSandbox
	}
	return ZarinpalBase
}

// zarinpalReply: data is an object on success and an empty list on failure,
// errors the other way round, so both stay raw until the code is known.
type zarinpalReply struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

func (r zarinpalReply) into(v any) error {
	if err := json.Unmarshal(r.Data, v); err != nil {
		return fmt.Errorf("zarinpal: %s", r.Errors)
	}
	return nil
}

func (g zarinpal) create(ctx context.Context, o *proto.PayOrder, callback string) (string, string, error) {
	body := map[string]any{
		"merchant_id": o.Key, "amount": o.AmountRial, "currency": "IRR",
		"callback_url": callback, "description": o.Desc,
	}
	if mobileRe.MatchString(o.Phone) {
		body["metadata"] = map[string]string{"mobile": o.Phone, "order_id": o.OrderID}
	} else {
		body["metadata"] = map[string]string{"order_id": o.OrderID}
	}
	var r zarinpalReply
	if err := postJSON(ctx, g.base(o)+"/pg/v4/payment/request.json", nil, body, &r); err != nil {
		return "", "", err
	}
	var d struct {
		Code      int    `json:"code"`
		Authority string `json:"authority"`
	}
	if err := r.into(&d); err != nil {
		return "", "", err
	}
	if d.Code != 100 || d.Authority == "" {
		return "", "", fmt.Errorf("zarinpal code %d: %s", d.Code, r.Errors)
	}
	return d.Authority, g.base(o) + "/pg/StartPay/" + d.Authority, nil
}

func (g zarinpal) verify(ctx context.Context, o *proto.PayOrder, f url.Values) (bool, string, error) {
	if f.Get("Authority") != o.Ref {
		return false, "", errForged
	}
	if f.Get("Status") != "OK" {
		return false, "", nil
	}
	var r zarinpalReply
	err := postJSON(ctx, g.base(o)+"/pg/v4/payment/verify.json", nil, map[string]any{
		"merchant_id": o.Key, "amount": o.AmountRial, "authority": o.Ref,
	}, &r)
	if err != nil {
		return false, "", err
	}
	var d struct {
		Code  int         `json:"code"`
		RefID json.Number `json:"ref_id"`
	}
	if err := r.into(&d); err != nil {
		return false, "", nil // not verified: zarinpal explains in errors
	}
	return d.Code == 100 || d.Code == 101, d.RefID.String(), nil
}

// ---------------------------------------------------------------- PayPing

type payping struct{}

func (payping) headers(o *proto.PayOrder) map[string]string {
	return map[string]string{"Authorization": "Bearer " + o.Key}
}

// PayPing counts in toman.
func (payping) toman(o *proto.PayOrder) int64 { return o.AmountRial / 10 }

func (g payping) create(ctx context.Context, o *proto.PayOrder, callback string) (string, string, error) {
	body := map[string]any{
		"amount": g.toman(o), "payerName": o.Name, "description": o.Desc,
		"returnUrl": callback, "clientRefId": o.OrderID,
	}
	if mobileRe.MatchString(o.Phone) {
		body["payerIdentity"] = o.Phone
	}
	var r struct {
		Code string `json:"code"`
	}
	if err := postJSON(ctx, PayPingBase+"/pay", g.headers(o), body, &r); err != nil {
		return "", "", err
	}
	if r.Code == "" {
		return "", "", errors.New("payping returned no payment code")
	}
	return r.Code, PayPingBase + "/pay/gotoipg/" + r.Code, nil
}

func (g payping) verify(ctx context.Context, o *proto.PayOrder, f url.Values) (bool, string, error) {
	refID := f.Get("refid")
	if f.Get("clientrefid") != o.OrderID {
		return false, "", errForged
	}
	if refID == "" { // no reference: the customer cancelled
		return false, "", nil
	}
	var v struct {
		Amount json.Number `json:"amount"`
	}
	// A 200 answer is PayPing's confirmation; anything else is an error.
	if err := postJSON(ctx, PayPingBase+"/pay/verify", g.headers(o), map[string]any{"refId": refID, "amount": g.toman(o)}, &v); err != nil {
		return false, refID, nil
	}
	if a, err := v.Amount.Int64(); err == nil && a != g.toman(o) {
		return false, refID, nil
	}
	return true, refID, nil
}

// ---------------------------------------------------------------- hub calls

// hubCall posts JSON to one of the hub's gate-only endpoints.
func (gt *Gate) hubCall(ctx context.Context, path string, in any, r *http.Request, out any) error {
	b, _ := json.Marshal(in)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, gt.hubURL(path), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+gt.g.GateID+"."+gt.g.Secret)
	if r != nil {
		// The customer's own session, so the hub knows whose order this is.
		if c := r.Header.Get("Cookie"); c != "" {
			req.Header.Set("Cookie", c)
		}
		req.Header.Set(proto.HeaderClientIP, clientIP(r).String())
	}
	resp, err := gt.hubHTTP.Do(req)
	if err != nil {
		return errHub
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		return &hubError{resp.StatusCode, e.Error}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	}
	return nil
}

var errHub = errors.New("hub_unreachable")

type hubError struct {
	status int
	code   string
}

func (e *hubError) Error() string { return e.code }

// ---------------------------------------------------------------- handlers

// payStart: the customer picked a plan and a gateway. Create the order on the
// hub, open the payment at the gateway, and hand the browser its page.
func (p *portal) payStart(w http.ResponseWriter, r *http.Request) {
	web.SecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("X-Requested-With") != "gozar" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var in struct {
		PlanID  int64  `json:"plan_id"`
		Gateway string `json:"gateway"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	gw, known := gateways[in.Gateway]
	if !known {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "online_pay_off"})
		return
	}
	ctx := r.Context()
	var o proto.PayOrder
	if err := p.gt.hubCall(ctx, "/v1/pay/start", in, r, &o); err != nil {
		var he *hubError
		if errors.As(err, &he) {
			writeJSON(w, he.status, map[string]string{"error": he.code})
		} else {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "hub_unreachable"})
		}
		return
	}
	callback := "https://" + r.Host + "/api/pay/callback/" + in.Gateway + "?o=" + url.QueryEscape(o.OrderID)
	ref, link, err := gw.create(ctx, &o, callback)
	if err != nil {
		p.gt.log.Warn("payment: could not open", "gateway", in.Gateway, "order", o.OrderID, "err", err)
		_ = p.gt.hubCall(ctx, "/v1/pay/finish", map[string]any{"order_id": o.OrderID, "ok": false, "note": "gateway error"}, nil, nil)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gateway_error"})
		return
	}
	if err := p.gt.hubCall(ctx, "/v1/pay/ref", map[string]string{"order_id": o.OrderID, "ref": ref}, nil, nil); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "hub_unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"link": link})
}

// payCallback: the gateway sends the customer back here, by GET or POST.
func (p *portal) payCallback(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	back := func(result string) {
		http.Redirect(w, r, "/plans?pay="+result, http.StatusSeeOther)
	}
	gw, known := gateways[r.PathValue("gateway")]
	orderID := r.URL.Query().Get("o")
	if !known || orderID == "" {
		back("fail")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var o proto.PayOrder
	if err := p.gt.hubCall(ctx, "/v1/pay/lookup", map[string]string{"order_id": orderID}, nil, &o); err != nil {
		back("error")
		return
	}
	if o.Gateway != r.PathValue("gateway") || o.Ref == "" {
		back("fail")
		return
	}
	switch o.Status {
	case "started":
	case "approved":
		back("ok") // already done: a reload, or the gateway calling twice
		return
	default:
		back("fail")
		return
	}
	ok, track, err := gw.verify(ctx, &o, r.Form)
	if errors.Is(err, errForged) {
		// Not about this payment. Ignore it rather than let it cancel
		// somebody's checkout.
		back("fail")
		return
	}
	if err != nil {
		// The gateway could not be asked. Leave the order open: the
		// customer can come back to this page and it will be tried again.
		p.gt.log.Warn("payment: verify failed", "gateway", o.Gateway, "order", orderID, "err", err)
		back("error")
		return
	}
	note := ""
	if !ok {
		note = "not paid"
	}
	if err := p.gt.hubCall(ctx, "/v1/pay/finish", map[string]any{"order_id": orderID, "ok": ok, "track_id": track, "note": note}, nil, nil); err != nil {
		back("error")
		return
	}
	if ok {
		p.gt.log.Info("payment verified", "gateway", o.Gateway, "order", orderID, "track", track)
		back("ok")
	} else {
		back("fail")
	}
}
