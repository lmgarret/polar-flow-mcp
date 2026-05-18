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
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:sec"))
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			t.Errorf("Authorization header = %q, want %q", got, expectedAuth)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}

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
			"access_token":  "tok",
			"token_type":    "bearer",
			"refresh_token": "ref",
			"expires_in":    43200,
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
	if got.TokenType != "bearer" {
		t.Errorf("TokenType = %q, want bearer", got.TokenType)
	}
	if got.RefreshToken != "ref" {
		t.Errorf("RefreshToken = %q, want ref", got.RefreshToken)
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
