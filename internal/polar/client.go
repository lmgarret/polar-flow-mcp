// Package polar provides a typed HTTP client for the Polar AccessLink v3 API.
package polar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a typed HTTP client for the Polar AccessLink v3 API.
// Created fresh per tool invocation; never cached at server or session level.
type Client struct {
	httpClient  *http.Client
	bearerToken string
}

// NewClient creates a new Polar API client for the given bearer token.
func NewClient(bearerToken string) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		bearerToken: bearerToken,
	}
}

//nolint:gochecknoglobals
var (
	// tokenEndpoint is the Polar OAuth2 token exchange URL. Overridable in tests.
	tokenEndpoint = "https://polarremote.com/v2/oauth2/token"

	// registerEndpoint is the Polar AccessLink user registration URL. Overridable in tests.
	registerEndpoint = "https://www.polaraccesslink.com/v3/users"

	// trainingTargetsBaseURL is the Polar AccessLink v3 training targets base URL.
	// Used as: trainingTargetsBaseURL + "/" + polarUserID + "/training-targets" (per CONTEXT.md D-01).
	// ASSUMED endpoint — not in public Polar v3 swagger; validate during live integration testing.
	trainingTargetsBaseURL = "https://www.polaraccesslink.com/v3/users"

	// defaultHTTPClient is shared by package-level functions (ExchangeCode, RegisterUser).
	defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// TokenResponse holds the fields returned by the Polar token endpoint.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	XUserID     int64  `json:"x_user_id"`
}

// ExchangeCode exchanges a Polar authorization code for an access token (per OAUTH-03).
// Uses HTTP Basic auth with client credentials, form-encoded body. Caller-supplied ctx
// satisfies the noctx linter via http.NewRequestWithContext.
func ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURL string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("polar: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polar: token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("polar: token exchange: status %d: %s", resp.StatusCode, body)
	}
	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("polar: decode token response: %w", err)
	}
	return &tr, nil
}

// RegisterUser registers the access token's owner with Polar AccessLink (per OAUTH-04).
// memberID is the partner's user identifier — must be the Polar numeric user ID
// (TokenResponse.XUserID, formatted as a string) per CR-02. The OAuth2 access token
// is used ONLY in the Authorization header, never in the body.
//
// HTTP 409 indicates the user is already registered; treat as idempotent success and
// return (0, nil). Callers MUST use the x_user_id from TokenResponse as the authoritative
// polar_user_id in all cases (200 and 409) — the 409 body does not contain the user object.
func RegisterUser(ctx context.Context, accessToken, memberID string) (int64, error) {
	body, err := json.Marshal(map[string]string{"member-id": memberID})
	if err != nil {
		return 0, fmt.Errorf("polar: marshal register body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerEndpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("polar: build register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("polar: register user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusConflict {
		return 0, nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("polar: register user: status %d: %s", resp.StatusCode, errBody)
	}
	var reg struct {
		PolarUserID int64 `json:"polar-user-id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return 0, fmt.Errorf("polar: decode register response: %w", err)
	}
	return reg.PolarUserID, nil
}

// CreateTrainingTargetRequest is the POST body for /v3/users/{id}/training-targets.
// Field names are ASSUMED from Polar v4 swagger schema (see RESEARCH.md A1-A8).
type CreateTrainingTargetRequest struct {
	Session  TrainingSessionTarget `json:"session"`
	Exercise []ExerciseTarget      `json:"exercise"`
}

// TrainingSessionTarget is the session container (name + scheduled start time).
type TrainingSessionTarget struct {
	Name      string   `json:"name"`
	StartTime DateTime `json:"startTime"`
}

// DateTime is the Polar nested date-time object (NOT ISO 8601 string).
type DateTime struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
	Hour  int `json:"hour"`
	Min   int `json:"min"`
	Sec   int `json:"sec"`
}

// ExerciseTarget is one workout block. type="PHASED" for interval workouts.
type ExerciseTarget struct {
	Idx           int64           `json:"idx"`
	Type          string          `json:"type"`
	PhaseOrRepeat []PhaseOrRepeat `json:"phaseOrRepeat,omitempty"`
}

// PhaseOrRepeat represents either a single phase (no repeatCount) or a repeat
// node (repeatCount set + nested PhaseOrRepeat children). The flat-to-tree
// transform in mcp_create_training_target.go builds this structure.
type PhaseOrRepeat struct {
	Name          string          `json:"name"`
	ChangeType    string          `json:"changeType"`
	Goal          PhaseGoal       `json:"goal"`
	Intensity     *PhaseIntensity `json:"intensity,omitempty"`
	RepeatCount   *int            `json:"repeatCount,omitempty"`
	PhaseOrRepeat []PhaseOrRepeat `json:"phaseOrRepeat,omitempty"`
}

// PhaseGoal: Type is "DURATION" (with Duration in milliseconds) | "DISTANCE"
// (with Distance in meters) | "MANUAL" (no duration/distance — for repeat nodes).
type PhaseGoal struct {
	Type     string   `json:"type"`
	Duration *int64   `json:"duration,omitempty"`
	Distance *float64 `json:"distance,omitempty"`
}

// PhaseIntensity: Type is "HEART_RATE_ZONES" with lower/upper zones 1-5, or "NONE".
type PhaseIntensity struct {
	Type      string `json:"type"`
	LowerZone *int   `json:"lowerZone,omitempty"`
	UpperZone *int   `json:"upperZone,omitempty"`
}

// ErrTargetNotFound is returned by DeleteTrainingTarget (and may be returned by
// future read methods) when the Polar API responds 404. Allows handlers to
// distinguish a missing-target case from genuine API/transport failures via
// errors.Is(err, polar.ErrTargetNotFound).
//
//nolint:gochecknoglobals
var ErrTargetNotFound = errors.New("polar: training target not found")

// TrainingTargetSummary is a minimal projection of a Polar training target
// suitable for human-readable listing. Only fields we can extract reliably
// across the assumed v3/v4 shapes are included.
type TrainingTargetSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Date string `json:"date"`
	Time string `json:"time"`
}

// CreateTrainingTarget POSTs a training target to Polar (per MCP-03).
// Returns the created target's id from the response body. ASSUMED endpoint and
// body shape — validate during live testing.
func (c *Client) CreateTrainingTarget(ctx context.Context, polarUserID string, body CreateTrainingTargetRequest) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("polar: marshal training target: %w", err)
	}
	reqURL := trainingTargetsBaseURL + "/" + polarUserID + "/training-targets"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("polar: build create training target request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("polar: create training target: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("polar: create training target: status %d: %s", resp.StatusCode, errBody)
	}
	// Try to parse {"id": ...} from response; tolerate string OR uint64 OR missing.
	var parsed struct {
		ID json.Number `json:"id"`
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if len(bodyBytes) > 0 {
		dec := json.NewDecoder(bytes.NewReader(bodyBytes))
		dec.UseNumber()
		_ = dec.Decode(&parsed)
		if parsed.ID.String() != "" {
			return parsed.ID.String(), nil
		}
	}
	return "(id not returned)", nil
}

// ListTrainingTargets fetches training targets in [fromDate, toDate] (ISO 8601
// YYYY-MM-DD). Returns a (possibly empty) slice. ASSUMED endpoint — validate live.
func (c *Client) ListTrainingTargets(ctx context.Context, polarUserID, fromDate, toDate string) ([]TrainingTargetSummary, error) {
	u, err := url.Parse(trainingTargetsBaseURL + "/" + polarUserID + "/training-targets")
	if err != nil {
		return nil, fmt.Errorf("polar: build list training targets URL: %w", err)
	}
	q := u.Query()
	if fromDate != "" {
		q.Set("from_date", fromDate)
	}
	if toDate != "" {
		q.Set("to_date", toDate)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("polar: build list training targets request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polar: list training targets: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("polar: list training targets: status %d: %s", resp.StatusCode, errBody)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("polar: read list response: %w", err)
	}
	slog.Debug("polar: list training targets raw response", "status", resp.StatusCode, "body", string(body))
	// Try array shape first.
	var direct []TrainingTargetSummary
	if json.Unmarshal(body, &direct) == nil {
		return direct, nil
	}
	// Fall back to object-with-"training-targets" shape (v4 swagger wraps the array).
	var wrapped struct {
		TrainingTargets []TrainingTargetSummary `json:"training-targets"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("polar: decode list response: %w", err)
	}
	if wrapped.TrainingTargets == nil {
		return []TrainingTargetSummary{}, nil
	}
	return wrapped.TrainingTargets, nil
}

// DeleteTrainingTarget DELETEs a training target by id. Returns nil on 200/204,
// ErrTargetNotFound on 404, and a wrapped error otherwise.
func (c *Client) DeleteTrainingTarget(ctx context.Context, polarUserID, targetID string) error {
	if targetID == "" {
		return fmt.Errorf("polar: delete training target: target_id is empty")
	}
	u := trainingTargetsBaseURL + "/" + polarUserID + "/training-targets/" + url.PathEscape(targetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("polar: build delete training target request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("polar: delete training target: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return ErrTargetNotFound
	default:
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("polar: delete training target: status %d: %s", resp.StatusCode, errBody)
	}
}
