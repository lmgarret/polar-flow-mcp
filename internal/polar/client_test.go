package polar_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/lm/polar-flow-mcp/internal/polar"
)

func TestExchangeCode_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assert method
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Assert Authorization header is Basic base64("cid:sec")
		expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:sec"))
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			t.Errorf("Authorization header = %q, want %q", got, expectedAuth)
		}

		// Assert Content-Type
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}

		// Assert body
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		if vals.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", vals.Get("grant_type"))
		}
		if vals.Get("code") != "code" {
			t.Errorf("code = %q, want code", vals.Get("code"))
		}
		if vals.Get("redirect_uri") != "https://r/cb" {
			t.Errorf("redirect_uri = %q, want https://r/cb", vals.Get("redirect_uri"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok",
			"token_type":   "bearer",
			"x_user_id":    42,
		})
	}))
	defer ts.Close()

	restore := polar.SetTokenEndpoint(ts.URL)
	t.Cleanup(restore)

	got, err := polar.ExchangeCode(context.Background(), "cid", "sec", "code", "https://r/cb")
	if err != nil {
		t.Fatalf("ExchangeCode returned error: %v", err)
	}
	if got.AccessToken != "tok" {
		t.Errorf("AccessToken = %q, want tok", got.AccessToken)
	}
	if got.XUserID != 42 {
		t.Errorf("XUserID = %d, want 42", got.XUserID)
	}
	if got.TokenType != "bearer" {
		t.Errorf("TokenType = %q, want bearer", got.TokenType)
	}
}

func TestExchangeCode_NonOKStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, "bad request")
	}))
	defer ts.Close()

	restore := polar.SetTokenEndpoint(ts.URL)
	t.Cleanup(restore)

	got, err := polar.ExchangeCode(context.Background(), "cid", "sec", "code", "https://r/cb")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil TokenResponse, got %+v", got)
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("error %q should contain 'status 400'", err.Error())
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Errorf("error %q should contain 'bad request'", err.Error())
	}
}

func TestRegisterUser_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assert Authorization header carries the access token (not the member-id).
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want 'Bearer tok'", got)
		}

		// Assert body has member-id equal to the explicit memberID parameter (not the access token).
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("body not valid JSON: %v", err)
		}
		// CR-02 regression guard: member-id must be "12345", NOT the access token "tok".
		if payload["member-id"] == "tok" {
			t.Errorf("member-id must not be the access token (CR-02); got %q", payload["member-id"])
		}
		if payload["member-id"] != "12345" {
			t.Errorf("member-id = %q, want \"12345\" (CR-02 regression)", payload["member-id"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"polar-user-id": 99,
		})
	}))
	defer ts.Close()

	restore := polar.SetRegisterEndpoint(ts.URL)
	t.Cleanup(restore)

	id, err := polar.RegisterUser(context.Background(), "tok", "12345")
	if err != nil {
		t.Fatalf("RegisterUser returned error: %v", err)
	}
	if id != 99 {
		t.Errorf("polar-user-id = %d, want 99", id)
	}
}

func TestRegisterUser_Conflict(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer ts.Close()

	restore := polar.SetRegisterEndpoint(ts.URL)
	t.Cleanup(restore)

	id, err := polar.RegisterUser(context.Background(), "tok", "12345")
	if err != nil {
		t.Fatalf("RegisterUser conflict should return nil error, got: %v", err)
	}
	if id != 0 {
		t.Errorf("expected id=0 on conflict, got %d", id)
	}
}

func TestRegisterUser_OtherError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "internal error")
	}))
	defer ts.Close()

	restore := polar.SetRegisterEndpoint(ts.URL)
	t.Cleanup(restore)

	id, err := polar.RegisterUser(context.Background(), "tok", "12345")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if id != 0 {
		t.Errorf("expected id=0 on error, got %d", id)
	}
}
