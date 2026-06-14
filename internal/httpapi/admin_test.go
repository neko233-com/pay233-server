package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neko233-com/pay233-server/internal/config"
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

	var session struct {
		Authenticated bool   `json:"authenticated"`
		Username      string `json:"username"`
		Role          string `json:"role"`
	}
	sessionReq := httptest.NewRequest(http.MethodGet, "/admin/api/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRec := httptest.NewRecorder()
	handler.ServeHTTP(sessionRec, sessionReq)
	if err := json.Unmarshal(sessionRec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if !session.Authenticated || session.Username != "root" || session.Role != "root" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestRootCreatesUsersAndEmployeeIsReadOnly(t *testing.T) {
	handler := testServer().Routes()
	rootCookie := loginCookie(t, handler, "root", "root")

	createEmployee := httptest.NewRequest(http.MethodPost, "/admin/api/users", bytes.NewReader([]byte(`{"username":"viewer","password":"viewer-pass","role":"employee"}`)))
	createEmployee.AddCookie(rootCookie)
	createEmployeeRec := httptest.NewRecorder()
	handler.ServeHTTP(createEmployeeRec, createEmployee)
	if createEmployeeRec.Code != http.StatusCreated {
		t.Fatalf("expected employee create 201, got %d: %s", createEmployeeRec.Code, createEmployeeRec.Body.String())
	}

	createAdmin := httptest.NewRequest(http.MethodPost, "/admin/api/users", bytes.NewReader([]byte(`{"username":"ops","password":"ops-pass","role":"admin"}`)))
	createAdmin.AddCookie(rootCookie)
	createAdminRec := httptest.NewRecorder()
	handler.ServeHTTP(createAdminRec, createAdmin)
	if createAdminRec.Code != http.StatusCreated {
		t.Fatalf("expected admin create 201, got %d: %s", createAdminRec.Code, createAdminRec.Body.String())
	}

	viewerCookie := loginCookie(t, handler, "viewer", "viewer-pass")
	dashboardReq := httptest.NewRequest(http.MethodGet, "/admin/api/dashboard", nil)
	dashboardReq.AddCookie(viewerCookie)
	dashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRec, dashboardReq)
	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("expected employee dashboard 200, got %d: %s", dashboardRec.Code, dashboardRec.Body.String())
	}

	markReq := httptest.NewRequest(http.MethodPost, "/admin/api/payments/missing/mark-lost", bytes.NewReader([]byte(`{}`)))
	markReq.AddCookie(viewerCookie)
	markRec := httptest.NewRecorder()
	handler.ServeHTTP(markRec, markReq)
	if markRec.Code != http.StatusForbidden {
		t.Fatalf("expected employee mark-lost 403, got %d: %s", markRec.Code, markRec.Body.String())
	}

	usersReq := httptest.NewRequest(http.MethodGet, "/admin/api/users", nil)
	usersReq.AddCookie(viewerCookie)
	usersRec := httptest.NewRecorder()
	handler.ServeHTTP(usersRec, usersReq)
	if usersRec.Code != http.StatusForbidden {
		t.Fatalf("expected employee users 403, got %d: %s", usersRec.Code, usersRec.Body.String())
	}

	healthReq := httptest.NewRequest(http.MethodPost, "/admin/api/channels/health-check", bytes.NewReader([]byte(`{}`)))
	healthReq.AddCookie(viewerCookie)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusForbidden {
		t.Fatalf("expected employee health-check 403, got %d: %s", healthRec.Code, healthRec.Body.String())
	}

	opsCookie := loginCookie(t, handler, "ops", "ops-pass")
	adminMarkReq := httptest.NewRequest(http.MethodPost, "/admin/api/payments/missing/mark-lost", bytes.NewReader([]byte(`{}`)))
	adminMarkReq.AddCookie(opsCookie)
	adminMarkRec := httptest.NewRecorder()
	handler.ServeHTTP(adminMarkRec, adminMarkReq)
	if adminMarkRec.Code != http.StatusNotFound {
		t.Fatalf("expected admin to pass role check and get 404, got %d: %s", adminMarkRec.Code, adminMarkRec.Body.String())
	}
}

func TestAuditLogAndRootPrune(t *testing.T) {
	handler := testServer().Routes()
	rootCookie := loginCookie(t, handler, "root", "root")

	auditReq := httptest.NewRequest(http.MethodGet, "/admin/api/audit?limit=10", nil)
	auditReq.AddCookie(rootCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit 200, got %d: %s", auditRec.Code, auditRec.Body.String())
	}
	if !bytes.Contains(auditRec.Body.Bytes(), []byte(`admin_login_success`)) {
		t.Fatalf("expected login audit entry, got %s", auditRec.Body.String())
	}

	createEmployee := httptest.NewRequest(http.MethodPost, "/admin/api/users", bytes.NewReader([]byte(`{"username":"viewer","password":"viewer-pass","role":"employee"}`)))
	createEmployee.AddCookie(rootCookie)
	createEmployeeRec := httptest.NewRecorder()
	handler.ServeHTTP(createEmployeeRec, createEmployee)
	if createEmployeeRec.Code != http.StatusCreated {
		t.Fatalf("expected employee create 201, got %d: %s", createEmployeeRec.Code, createEmployeeRec.Body.String())
	}
	viewerCookie := loginCookie(t, handler, "viewer", "viewer-pass")
	pruneAsViewer := httptest.NewRequest(http.MethodPost, "/admin/api/audit/prune", nil)
	pruneAsViewer.AddCookie(viewerCookie)
	pruneAsViewerRec := httptest.NewRecorder()
	handler.ServeHTTP(pruneAsViewerRec, pruneAsViewer)
	if pruneAsViewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected employee prune 403, got %d: %s", pruneAsViewerRec.Code, pruneAsViewerRec.Body.String())
	}

	pruneAsRoot := httptest.NewRequest(http.MethodPost, "/admin/api/audit/prune", nil)
	pruneAsRoot.AddCookie(rootCookie)
	pruneAsRootRec := httptest.NewRecorder()
	handler.ServeHTTP(pruneAsRootRec, pruneAsRoot)
	if pruneAsRootRec.Code != http.StatusOK {
		t.Fatalf("expected root prune 200, got %d: %s", pruneAsRootRec.Code, pruneAsRootRec.Body.String())
	}
}

func TestAdminChannelHealthCheck(t *testing.T) {
	handler := testServerWithChannels([]config.ChannelConfig{{
		Name:     "mock-down",
		Provider: "mock",
		Enabled:  true,
		Options:  map[string]string{"health_status": "down"},
	}}).Routes()
	rootCookie := loginCookie(t, handler, "root", "root")

	checkReq := httptest.NewRequest(http.MethodPost, "/admin/api/channels/health-check", bytes.NewReader([]byte(`{}`)))
	checkReq.AddCookie(rootCookie)
	checkRec := httptest.NewRecorder()
	handler.ServeHTTP(checkRec, checkReq)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("expected health check 200, got %d: %s", checkRec.Code, checkRec.Body.String())
	}
	if !bytes.Contains(checkRec.Body.Bytes(), []byte(`"health":"down"`)) {
		t.Fatalf("expected down health, got %s", checkRec.Body.String())
	}

	if !bytes.Contains(checkRec.Body.Bytes(), []byte(`"last_checked_at"`)) {
		t.Fatalf("expected health metadata, got %s", checkRec.Body.String())
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

func loginCookie(t *testing.T, handler http.Handler, username string, password string) *http.Cookie {
	t.Helper()
	body := []byte(`{"username":"` + username + `","password":"` + password + `"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(body))
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected login cookie")
	}
	return cookies[0]
}
