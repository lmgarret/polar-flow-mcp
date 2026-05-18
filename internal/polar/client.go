// Package polar provides a typed HTTP client for the Polar AccessLink API (v3 + v4).
package polar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a typed HTTP client for the Polar AccessLink API.
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
	// tokenEndpoint is the Polar v4 OAuth2 token exchange URL. Overridable in tests.
	tokenEndpoint = "https://auth.polar.com/oauth/token"

	// trainingTargetsV4URL is the Polar v4 calendar targets list endpoint.
	// Does not require a user ID in the path; uses fromDate/toDate query params.
	trainingTargetsV4URL = "https://www.polaraccesslink.com/v4/data/training-target/calendar-targets"

	// trainingTargetsV4BaseURL is the base for v4 training target mutation endpoints.
	trainingTargetsV4BaseURL = "https://www.polaraccesslink.com/v4/data/training-target"

	// defaultHTTPClient is shared by package-level functions (ExchangeCode).
	defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// TokenResponse holds the fields returned by the Polar v4 token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
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

// TrainingTargetSummary is a Polar v4 training target as returned by the calendar-targets endpoint.
type TrainingTargetSummary struct {
	Session  TrainingTargetSession    `json:"session"`
	Exercise []TrainingExerciseTarget `json:"exercise"`
}

// TrainingTargetSession holds the session-level fields of a v4 training target.
type TrainingTargetSession struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	StartTime   DateTime `json:"startTime"`
	Description string   `json:"description"`
}

// TrainingExerciseTarget holds per-exercise goal fields within a training target.
type TrainingExerciseTarget struct {
	Idx      int     `json:"idx"`
	Duration int     `json:"duration"`
	Distance float64 `json:"distance"`
}

// CreateTrainingTarget POSTs a training target to Polar (per MCP-03).
// Returns the created target's id from the response body. ASSUMED endpoint and
// body shape — validate during live testing.
func (c *Client) CreateTrainingTarget(ctx context.Context, polarUserID string, body CreateTrainingTargetRequest) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("polar: marshal training target: %w", err)
	}
	reqURL := trainingTargetsV4BaseURL + "/" + polarUserID + "/training-targets"
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

// ListTrainingTargets fetches calendar training targets from the Polar v4 API
// for the given date range (ISO 8601 YYYY-MM-DD, fromDate inclusive, toDate exclusive).
func (c *Client) ListTrainingTargets(ctx context.Context, fromDate, toDate string) ([]TrainingTargetSummary, error) {
	u, err := url.Parse(trainingTargetsV4URL)
	if err != nil {
		return nil, fmt.Errorf("polar: build list training targets URL: %w", err)
	}
	q := u.Query()
	if fromDate != "" {
		q.Set("from", fromDate)
	}
	if toDate != "" {
		q.Set("to", toDate)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("polar: read list response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("polar: list training targets: status %d: %s", resp.StatusCode, body)
	}

	var targets []TrainingTargetSummary
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("polar: decode list response: %w", err)
	}
	if targets == nil {
		return []TrainingTargetSummary{}, nil
	}
	return targets, nil
}

// DeleteTrainingTarget DELETEs a training target by id. Returns nil on 200/204,
// ErrTargetNotFound on 404, and a wrapped error otherwise.
func (c *Client) DeleteTrainingTarget(ctx context.Context, polarUserID, targetID string) error {
	if targetID == "" {
		return fmt.Errorf("polar: delete training target: target_id is empty")
	}
	u := trainingTargetsV4BaseURL + "/" + polarUserID + "/training-targets/" + url.PathEscape(targetID)
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
