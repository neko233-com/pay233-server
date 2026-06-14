package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminRequiresLogin(t *testing.T) {
	handler := testServer().Routes()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/dashboard", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAdminHTMLRouting(t *testing.T) {
	handler := testServer().Routes()

	rootReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rootRec := httptest.NewRecorder()
	handler.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusFound || rootRec.Header().Get("Location") != "/admin/login.html" {
		t.Fatalf("expected /admin redirect to login, got %d %q", rootRec.Code, rootRec.Header().Get("Location"))
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/admin/dashboard.html", nil)
	dashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRec, dashboardReq)
	if dashboardRec.Code != http.StatusFound || dashboardRec.Header().Get("Location") != "/admin/login.html" {
		t.Fatalf("expected dashboard redirect to login, got %d %q", dashboardRec.Code, dashboardRec.Header().Get("Location"))
	}
}

func TestAdminLoginAndDashboard(t *testing.T) {
	handler := testServer().Routes()
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader([]byte(`{"username":"root","password":"root"}`)))
	loginRec := httptest.NewRecorder()

	handler.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected admin cookie")
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/admin/dashboard.html", nil)
	pageReq.AddCookie(cookies[0])
	pageRec := httptest.NewRecorder()
	handler.ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("expected dashboard page 200, got %d", pageRec.Code)
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/admin/api/dashboard", nil)
	dashboardReq.AddCookie(cookies[0])
	dashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRec, dashboardReq)

	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", dashboardRec.Code, dashboardRec.Body.String())
	}
	var got struct {
		KPIs struct {
			TotalPayments int `json:"total_payments"`
		} `json:"kpis"`
		ChannelInfo []struct {
			Name string `json:"name"`
		} `json:"channel_info"`
	}
	if err := json.Unmarshal(dashboardRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ChannelInfo) == 0 {
		t.Fatal("expected channel info")
	}
}

func TestAdminMarkLost(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"envType":"test","merchant_id":"m1","out_trade_no":"lost-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test","notify_url":"https://merchant.test/notify"}`)
	createReq := signedRequest(http.MethodPost, "/v1/payments", body)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader([]byte(`{"username":"root","password":"root"}`)))
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	cookie := loginRec.Result().Cookies()[0]

	markReq := httptest.NewRequest(http.MethodPost, "/admin/api/payments/"+created.ID+"/mark-lost", bytes.NewReader([]byte(`{"reason":"gateway reconciliation missing"}`)))
	markReq.AddCookie(cookie)
	markRec := httptest.NewRecorder()
	handler.ServeHTTP(markRec, markReq)

	if markRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", markRec.Code, markRec.Body.String())
	}
	if !bytes.Contains(markRec.Body.Bytes(), []byte(`"status":"lost"`)) {
		t.Fatalf("expected lost payment, got %s", markRec.Body.String())
	}
}
