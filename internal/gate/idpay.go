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
	"strconv"
	"time"

	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/web"
)

// IDPayBase is IDPay's API. A variable so tests can point it elsewhere.
var IDPayBase = "https://api.idpay.ir/v1.1"

// IDPayClient calls IDPay. A variable so tests can trust a fake server.
var IDPayClient = &http.Client{Timeout: 20 * time.Second}

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

// idpay calls one IDPay endpoint.
func idpay(ctx context.Context, o *proto.IDPayOrder, path string, in, out any) error {
	b, _ := json.Marshal(in)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, IDPayBase+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", o.APIKey)
	if o.Sandbox {
		req.Header.Set("X-SANDBOX", "1")
	}
	resp, err := IDPayClient.Do(req)
	if err != nil {
		return fmt.Errorf("idpay unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		var e struct {
			Code    int    `json:"error_code"`
			Message string `json:"error_message"`
		}
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("idpay %d: %d %s", resp.StatusCode, e.Code, e.Message)
	}
	return json.Unmarshal(body, out)
}

// payStart: the customer chose a plan and "pay online". Create the order on
// the hub, open a payment at IDPay, and hand the browser IDPay's page.
func (p *portal) payStart(w http.ResponseWriter, r *http.Request) {
	web.SecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("X-Requested-With") != "gozar" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var in struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	ctx := r.Context()
	var o proto.IDPayOrder
	if err := p.gt.hubCall(ctx, "/v1/idpay/start", in, r, &o); err != nil {
		var he *hubError
		if errors.As(err, &he) {
			writeJSON(w, he.status, map[string]string{"error": he.code})
		} else {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "hub_unreachable"})
		}
		return
	}
	var created struct {
		ID   string `json:"id"`
		Link string `json:"link"`
	}
	err := idpay(ctx, &o, "/payment", map[string]any{
		"order_id": o.OrderID, "amount": o.AmountRial, "name": o.Name, "phone": o.Phone,
		"desc": o.Desc, "callback": "https://" + r.Host + "/api/pay/callback",
	}, &created)
	if err != nil || created.Link == "" {
		p.gt.log.Warn("idpay: could not open a payment", "order", o.OrderID, "err", err)
		_ = p.gt.hubCall(ctx, "/v1/idpay/finish", map[string]any{"order_id": o.OrderID, "ok": false, "note": "gateway error"}, nil, nil)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gateway_error"})
		return
	}
	_ = p.gt.hubCall(ctx, "/v1/idpay/ref", map[string]string{"order_id": o.OrderID, "ref": created.ID}, nil, nil)
	writeJSON(w, http.StatusOK, map[string]string{"link": created.Link})
}

// payCallback: IDPay sends the customer back here, by POST or GET. Verify
// with IDPay before believing anything the redirect says, record the
// outcome on the hub, and show the customer the result.
func (p *portal) payCallback(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f := r.Form
	orderID, payID := f.Get("order_id"), f.Get("id")
	status, _ := strconv.Atoi(f.Get("status"))
	back := func(result string) {
		http.Redirect(w, r, "/plans?pay="+url.QueryEscape(result), http.StatusSeeOther)
	}
	if orderID == "" {
		back("fail")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var o proto.IDPayOrder
	if err := p.gt.hubCall(ctx, "/v1/idpay/lookup", map[string]string{"order_id": orderID}, nil, &o); err != nil {
		back("error")
		return
	}
	if o.Status != "started" {
		if o.Status == "approved" {
			back("ok")
		} else {
			back("fail")
		}
		return
	}
	finish := func(ok bool, track, note string) {
		_ = p.gt.hubCall(ctx, "/v1/idpay/finish", map[string]any{"order_id": orderID, "ok": ok, "track_id": track, "note": note}, nil, nil)
	}
	// 10: paid and waiting for verification. Anything else means the
	// customer cancelled or the bank refused.
	// A callback naming a payment IDPay never issued for this order is not
	// from IDPay. Ignore it rather than let it cancel someone's checkout.
	if o.Ref != "" && payID != o.Ref {
		back("fail")
		return
	}
	if status != 10 {
		finish(false, f.Get("track_id"), "status "+f.Get("status"))
		back("fail")
		return
	}
	var v struct {
		Status  int         `json:"status"`
		TrackID json.Number `json:"track_id"`
		Amount  json.Number `json:"amount"`
		OrderID string      `json:"order_id"`
	}
	if err := idpay(ctx, &o, "/payment/verify", map[string]string{"id": payID, "order_id": orderID}, &v); err != nil {
		p.gt.log.Warn("idpay: verify failed", "order", orderID, "err", err)
		finish(false, f.Get("track_id"), "verify failed")
		back("fail")
		return
	}
	amount, _ := v.Amount.Int64()
	if (v.Status != 100 && v.Status != 101) || v.OrderID != orderID || amount != o.AmountRial {
		p.gt.log.Warn("idpay: verification did not match", "order", orderID, "status", v.Status, "amount", amount, "want", o.AmountRial)
		finish(false, v.TrackID.String(), "not verified")
		back("fail")
		return
	}
	finish(true, v.TrackID.String(), "")
	p.gt.log.Info("idpay: payment verified", "order", orderID, "track", v.TrackID.String())
	back("ok")
}
