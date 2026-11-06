package epay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applog "github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
)

func initQueryTestEnv(t *testing.T) {
	t.Helper()

	if err := applog.Init(filepath.Join(t.TempDir(), "logs")); err != nil {
		t.Fatalf("init log: %v", err)
	}
	t.Cleanup(applog.Close)

	if err := model.Init(filepath.Join(t.TempDir(), "epay-test.db"), ""); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if db, err := model.Db.DB(); err == nil {
			_ = db.Close()
		}
	})
}

func createQueryOrder(t *testing.T, orderId, tradeId string, status int, money string) {
	t.Helper()

	now := model.Datetime(time.Now())
	confirmed := time.Now()
	o := model.Order{
		OrderId:     orderId,
		TradeId:     tradeId,
		TradeType:   model.UsdtTrc20,
		Money:       money,
		Amount:      "1.234567",
		Status:      status,
		Name:        "test",
		ApiType:     model.OrderApiTypeEpay,
		ExpiredAt:   time.Now().Add(time.Hour),
		ConfirmedAt: &confirmed,
	}
	o.CreatedAt = &now
	o.UpdatedAt = &now
	if err := model.Db.Create(&o).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
}

type queryResp struct {
	Code       int    `json:"code"`
	Msg        string `json:"msg"`
	TradeNo    string `json:"trade_no"`
	OutTradeNo string `json:"out_trade_no"`
	Money      string `json:"money"`
	Status     int    `json:"status"`
}

func doQuery(t *testing.T, method string, query, form url.Values) queryResp {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := new(Epay)
	r.GET("/api.php", h.Api)
	r.POST("/api.php", h.Api)

	target := "/api.php"
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var req *http.Request
	if method == http.MethodPost {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("http status = %d", w.Code)
	}

	var resp queryResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}

	return resp
}

func params(extra map[string]string) url.Values {
	v := url.Values{
		"act": {"order"},
		"pid": {Pid},
		"key": {model.AuthToken()},
	}
	for k, val := range extra {
		v.Set(k, val)
	}

	return v
}

func TestApiOrderQueryRejectsEmptyToken(t *testing.T) {
	initQueryTestEnv(t)
	createQueryOrder(t, "paid-1", "T-PAID", model.OrderStatusSuccess, "10.5")

	model.Db.Model(&model.Conf{}).Where("k = ?", model.ApiAuthToken).Update("v", "")

	q := url.Values{"act": {"order"}, "pid": {Pid}, "out_trade_no": {"paid-1"}}
	if resp := doQuery(t, http.MethodGet, q, nil); resp.Code != -1 {
		t.Fatalf("missing key with empty token: code = %d, want -1", resp.Code)
	}

	q.Set("key", "")
	if resp := doQuery(t, http.MethodGet, q, nil); resp.Code != -1 {
		t.Fatalf("empty key with empty token: code = %d, want -1", resp.Code)
	}
}

func TestApiOrderQuery(t *testing.T) {
	initQueryTestEnv(t)

	createQueryOrder(t, "paid-1", "T-PAID", model.OrderStatusSuccess, "10.5")
	createQueryOrder(t, "wait-1", "T-WAIT", model.OrderStatusWaiting, "20")
	createQueryOrder(t, "conf-1", "T-CONF", model.OrderStatusConfirming, "30")
	createQueryOrder(t, "dup-1", "T-DUP-OLD", model.OrderStatusExpired, "40")
	createQueryOrder(t, "dup-1", "T-DUP-NEW", model.OrderStatusSuccess, "40")

	cases := []struct {
		name       string
		method     string
		query      url.Values
		form       url.Values
		wantCode   int
		wantStatus int
		wantTrade  string
		wantMoney  string
	}{
		{"paid via query", http.MethodGet, params(map[string]string{"out_trade_no": "paid-1"}), nil, 1, 1, "T-PAID", "10.5"},
		{"waiting", http.MethodGet, params(map[string]string{"out_trade_no": "wait-1"}), nil, 1, 0, "T-WAIT", "20"},
		{"confirming is not paid", http.MethodGet, params(map[string]string{"out_trade_no": "conf-1"}), nil, 1, 0, "T-CONF", "30"},
		{"paid via form body only", http.MethodPost, nil, params(map[string]string{"out_trade_no": "paid-1"}), 1, 1, "T-PAID", "10.5"},
		{"query by trade_no", http.MethodGet, params(map[string]string{"trade_no": "T-WAIT"}), nil, 1, 0, "T-WAIT", "20"},
		{"duplicate order_id returns latest", http.MethodGet, params(map[string]string{"out_trade_no": "dup-1"}), nil, 1, 1, "T-DUP-NEW", "40"},
		{"trade_no and out_trade_no mismatch", http.MethodGet, params(map[string]string{"trade_no": "T-PAID", "out_trade_no": "wait-1"}), nil, -1, 0, "", ""},
		{"not found", http.MethodGet, params(map[string]string{"out_trade_no": "nope"}), nil, -1, 0, "", ""},
		{"wrong key", http.MethodGet, params(map[string]string{"out_trade_no": "paid-1", "key": "bad"}), nil, -1, 0, "", ""},
		{"wrong pid", http.MethodGet, params(map[string]string{"out_trade_no": "paid-1", "pid": "9"}), nil, -1, 0, "", ""},
		{"unsupported act", http.MethodGet, params(map[string]string{"out_trade_no": "paid-1", "act": "refund"}), nil, -1, 0, "", ""},
		{"missing order identifiers", http.MethodGet, params(nil), nil, -1, 0, "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := doQuery(t, c.method, c.query, c.form)
			if resp.Code != c.wantCode {
				t.Fatalf("code = %d (%s), want %d", resp.Code, resp.Msg, c.wantCode)
			}
			if resp.Code != 1 {
				return
			}
			if resp.Status != c.wantStatus || resp.TradeNo != c.wantTrade || resp.Money != c.wantMoney {
				t.Fatalf("got status=%d trade_no=%s money=%s, want status=%d trade_no=%s money=%s",
					resp.Status, resp.TradeNo, resp.Money, c.wantStatus, c.wantTrade, c.wantMoney)
			}
		})
	}
}
