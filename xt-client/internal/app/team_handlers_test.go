package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaotao/xt-client/internal/office"
)

func TestTeamRoutesOriginValidationAndRevisionConflict(t *testing.T) {
	svc, err := office.New(t.TempDir(), &officeFakeExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	mux := http.NewServeMux()
	registerOfficeRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("Origin") == "http://office.test" }, func(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }, func(w http.ResponseWriter, code int, message string) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
	})
	request := office.TeamRequest{ProjectPath: t.TempDir(), TemplateID: "product-delivery", Roles: office.TeamTemplates()[0].Roles}
	body, _ := json.Marshal(request)
	blocked := httptest.NewRecorder()
	mux.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/api/office/team/bootstrap", bytes.NewReader(body)))
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("origin: %d", blocked.Code)
	}
	created := httptest.NewRecorder()
	post := httptest.NewRequest(http.MethodPost, "/api/office/team/bootstrap", bytes.NewReader(body))
	post.Header.Set("Origin", "http://office.test")
	mux.ServeHTTP(created, post)
	if created.Code != http.StatusOK {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	conflict := httptest.NewRecorder()
	post = httptest.NewRequest(http.MethodPost, "/api/office/team/bootstrap", bytes.NewReader(body))
	post.Header.Set("Origin", "http://office.test")
	mux.ServeHTTP(conflict, post)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("stale: %d %s", conflict.Code, conflict.Body.String())
	}
	unknown := httptest.NewRecorder()
	post = httptest.NewRequest(http.MethodPost, "/api/office/roles", bytes.NewBufferString(`{"unknown":true}`))
	post.Header.Set("Origin", "http://office.test")
	mux.ServeHTTP(unknown, post)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("strict JSON: %d", unknown.Code)
	}
}

func TestSkipOnboardingRoute(t *testing.T) {
	svc, err := office.New(t.TempDir(), &officeFakeExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	mux := http.NewServeMux()
	registerOfficeRoutes(mux, svc, func(r *http.Request) bool { return r.Header.Get("Origin") == "http://office.test" }, func(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }, func(w http.ResponseWriter, code int, message string) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
	})
	body, _ := json.Marshal(office.TeamRequest{ProjectPath: t.TempDir()})
	request := httptest.NewRequest(http.MethodPost, "/api/office/team/skip-onboarding", bytes.NewReader(body))
	request.Header.Set("Origin", "http://office.test")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("skip: %d %s", response.Code, response.Body.String())
	}
	var snapshot office.TeamSnapshot
	if err = json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil || snapshot.Onboarding != office.OnboardingSkipped {
		t.Fatalf("skip result: %+v, %v", snapshot, err)
	}
}
