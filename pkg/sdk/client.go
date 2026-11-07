package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIError represents an error returned by the Kerob HTTP API.
type APIError struct {
	StatusCode int    `json:"status_code"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	Details    any    `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("kerob api error (%d %s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("kerob api error (%d): %s", e.StatusCode, e.Message)
}

// Option configures a Client instance.
type Option func(*Client)

// WithToken configures the Bearer authentication token.
func WithToken(token string) Option {
	return func(c *Client) {
		c.token = strings.TrimSpace(token)
	}
}

// WithTenant configures the default tenant context header.
func WithTenant(tenantID string) Option {
	return func(c *Client) {
		c.tenantID = strings.TrimSpace(tenantID)
	}
}

// WithHTTPClient configures a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithUserAgent configures a custom User-Agent header string.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		c.userAgent = ua
	}
}

// Client provides typed programmatic access to the Kerob Automation Engine.
type Client struct {
	endpoint   string
	token      string
	tenantID   string
	userAgent  string
	httpClient *http.Client
}

// NewClient constructs an initialized Client.
func NewClient(endpoint string, opts ...Option) (*Client, error) {
	if endpoint == "" {
		endpoint = "http://localhost:8080"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid kerob endpoint URL %q: %w", endpoint, err)
	}

	c := &Client{
		endpoint:   endpoint,
		userAgent:  "Kerob-Go-SDK/1.0",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Endpoint returns the configured base URL.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// Token returns the configured auth token.
func (c *Client) Token() string {
	return c.token
}

// TenantID returns the configured tenant ID.
func (c *Client) TenantID() string {
	return c.tenantID
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body any, target any) error {
	fullURL := fmt.Sprintf("%s%s", c.endpoint, path)
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		switch v := body.(type) {
		case []byte:
			reqBody = bytes.NewReader(v)
		case string:
			reqBody = strings.NewReader(v)
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("failed to serialize request payload: %w", err)
			}
			reqBody = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to build HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.tenantID != "" {
		req.Header.Set("X-Tenant-ID", c.tenantID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s %s failed: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		var parsed map[string]any
		if jsonErr := json.Unmarshal(respBytes, &parsed); jsonErr == nil {
			if msg, ok := parsed["error"].(string); ok {
				apiErr.Message = msg
			} else if msg, ok := parsed["message"].(string); ok {
				apiErr.Message = msg
			}
			if code, ok := parsed["code"].(string); ok {
				apiErr.Code = code
			}
			apiErr.Details = parsed["details"]
		}
		if apiErr.Message == "" {
			apiErr.Message = string(respBytes)
		}
		return apiErr
	}

	if target != nil && len(respBytes) > 0 {
		if rawTarget, ok := target.(*[]byte); ok {
			*rawTarget = respBytes
			return nil
		}
		if err := json.Unmarshal(respBytes, target); err != nil {
			return fmt.Errorf("failed to decode response into target type: %w", err)
		}
	}
	return nil
}

// --- Auth & Tenant APIs ---

// GetMe retrieves identity metadata for the active credentials.
func (c *Client) GetMe(ctx context.Context) (*Identity, error) {
	var res Identity
	if err := c.request(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ListTenants lists organizational tenants.
func (c *Client) ListTenants(ctx context.Context) ([]Tenant, error) {
	var res []Tenant
	if err := c.request(ctx, http.MethodGet, "/api/v1/tenants", nil, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetTenant retrieves a tenant by ID.
func (c *Client) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	var res Tenant
	if err := c.request(ctx, http.MethodGet, "/api/v1/tenants/"+url.PathEscape(id), nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// CreateTenant creates a new organizational tenant.
func (c *Client) CreateTenant(ctx context.Context, name, slug string) (*Tenant, error) {
	payload := map[string]string{"name": name, "slug": slug}
	var res Tenant
	if err := c.request(ctx, http.MethodPost, "/api/v1/tenants", nil, payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ListMembers lists members of the current tenant.
func (c *Client) ListMembers(ctx context.Context) ([]Member, error) {
	var res []Member
	if err := c.request(ctx, http.MethodGet, "/api/v1/members", nil, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// ListAPITokens lists scoped API credentials.
func (c *Client) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	var res []APIToken
	if err := c.request(ctx, http.MethodGet, "/api/v1/tokens", nil, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// CreateAPIToken generates a new scoped API token.
func (c *Client) CreateAPIToken(ctx context.Context, name string, scopes []string) (*APIToken, error) {
	payload := map[string]any{"name": name, "scopes": scopes}
	var res APIToken
	if err := c.request(ctx, http.MethodPost, "/api/v1/tokens", nil, payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// RevokeAPIToken revokes an existing API token.
func (c *Client) RevokeAPIToken(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/tokens/"+url.PathEscape(id), nil, nil, nil)
}

// --- Workflow APIs ---

// WorkflowFilter defines query parameters for listing workflows.
type WorkflowFilter struct {
	Enabled *bool
	Tag     string
	Limit   int
	Offset  int
}

// ListWorkflows retrieves workflows matching optional criteria.
func (c *Client) ListWorkflows(ctx context.Context, filters ...WorkflowFilter) ([]Workflow, error) {
	q := url.Values{}
	if len(filters) > 0 {
		f := filters[0]
		if f.Enabled != nil {
			q.Set("enabled", strconv.FormatBool(*f.Enabled))
		}
		if f.Tag != "" {
			q.Set("tag", f.Tag)
		}
		if f.Limit > 0 {
			q.Set("limit", strconv.Itoa(f.Limit))
		}
		if f.Offset > 0 {
			q.Set("offset", strconv.Itoa(f.Offset))
		}
	}
	var res []Workflow
	if err := c.request(ctx, http.MethodGet, "/api/v1/workflows", q, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetWorkflow fetches a workflow by ID.
func (c *Client) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	var res Workflow
	if err := c.request(ctx, http.MethodGet, "/api/v1/workflows/"+url.PathEscape(id), nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// CreateWorkflow creates a new workflow definition.
func (c *Client) CreateWorkflow(ctx context.Context, input WorkflowCreateInput) (*Workflow, error) {
	var res Workflow
	if err := c.request(ctx, http.MethodPost, "/api/v1/workflows", nil, input, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// UpdateWorkflow performs a full update of an existing workflow.
func (c *Client) UpdateWorkflow(ctx context.Context, id string, input WorkflowCreateInput) (*Workflow, error) {
	var res Workflow
	if err := c.request(ctx, http.MethodPut, "/api/v1/workflows/"+url.PathEscape(id), nil, input, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// PatchWorkflow partially updates an existing workflow (e.g. enabling or changing description).
func (c *Client) PatchWorkflow(ctx context.Context, id string, patch map[string]any) (*Workflow, error) {
	var res Workflow
	if err := c.request(ctx, http.MethodPatch, "/api/v1/workflows/"+url.PathEscape(id), nil, patch, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// DeleteWorkflow removes a workflow definition.
func (c *Client) DeleteWorkflow(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/workflows/"+url.PathEscape(id), nil, nil, nil)
}

// TestWorkflow dry-runs a test execution of a workflow against sample event payload.
func (c *Client) TestWorkflow(ctx context.Context, id string, payload map[string]any) (map[string]any, error) {
	var res map[string]any
	if err := c.request(ctx, http.MethodPost, "/api/v1/workflows/"+url.PathEscape(id)+"/test", nil, payload, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// ListWorkflowVersions returns the version history of a workflow.
func (c *Client) ListWorkflowVersions(ctx context.Context, id string) ([]WorkflowVersion, error) {
	var res []WorkflowVersion
	if err := c.request(ctx, http.MethodGet, "/api/v1/workflows/"+url.PathEscape(id)+"/versions", nil, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetWorkflowVersion fetches a specific version snapshot of a workflow.
func (c *Client) GetWorkflowVersion(ctx context.Context, id string, version int) (*WorkflowVersion, error) {
	var res WorkflowVersion
	path := fmt.Sprintf("/api/v1/workflows/%s/versions/%d", url.PathEscape(id), version)
	if err := c.request(ctx, http.MethodGet, path, nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// RollbackWorkflow rolls back a workflow configuration to a historical version.
func (c *Client) RollbackWorkflow(ctx context.Context, id string, version int) (*Workflow, error) {
	var res struct {
		Workflow Workflow `json:"workflow"`
	}
	path := fmt.Sprintf("/api/v1/workflows/%s/rollback/%d", url.PathEscape(id), version)
	if err := c.request(ctx, http.MethodPost, path, nil, nil, &res); err != nil {
		return nil, err
	}
	return &res.Workflow, nil
}

// DiffWorkflowVersions returns diff metadata between two historical versions.
func (c *Client) DiffWorkflowVersions(ctx context.Context, id string, from, to int) (*WorkflowDiff, error) {
	q := url.Values{
		"from": {strconv.Itoa(from)},
		"to":   {strconv.Itoa(to)},
	}
	var res WorkflowDiff
	path := fmt.Sprintf("/api/v1/workflows/%s/diff", url.PathEscape(id))
	if err := c.request(ctx, http.MethodGet, path, q, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ExportWorkflow exports a workflow in JSON or YAML format.
func (c *Client) ExportWorkflow(ctx context.Context, id string, format string) ([]byte, error) {
	q := url.Values{}
	if format != "" {
		q.Set("format", format)
	}
	var res []byte
	path := fmt.Sprintf("/api/v1/workflows/%s/export", url.PathEscape(id))
	if err := c.request(ctx, http.MethodGet, path, q, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// ImportWorkflow imports a workflow from serialized definition.
func (c *Client) ImportWorkflow(ctx context.Context, data []byte) (*Workflow, error) {
	var res Workflow
	if err := c.request(ctx, http.MethodPost, "/api/v1/workflows/import", nil, data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// --- Execution APIs ---

// ExecutionFilter defines query parameters for listing executions.
type ExecutionFilter struct {
	WorkflowID string
	Status     string
	Limit      int
	Offset     int
}

// ListExecutions retrieves workflow execution runs.
func (c *Client) ListExecutions(ctx context.Context, filters ...ExecutionFilter) ([]Execution, error) {
	q := url.Values{}
	if len(filters) > 0 {
		f := filters[0]
		if f.WorkflowID != "" {
			q.Set("workflow_id", f.WorkflowID)
		}
		if f.Status != "" {
			q.Set("status", f.Status)
		}
		if f.Limit > 0 {
			q.Set("limit", strconv.Itoa(f.Limit))
		}
		if f.Offset > 0 {
			q.Set("offset", strconv.Itoa(f.Offset))
		}
	}
	var res []Execution
	if err := c.request(ctx, http.MethodGet, "/api/v1/executions", q, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetExecution fetches detailed execution metadata and step history.
func (c *Client) GetExecution(ctx context.Context, id string) (*Execution, error) {
	var res Execution
	if err := c.request(ctx, http.MethodGet, "/api/v1/executions/"+url.PathEscape(id), nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// RetryExecution triggers a retry of a failed execution.
func (c *Client) RetryExecution(ctx context.Context, id string) (*Execution, error) {
	var res Execution
	if err := c.request(ctx, http.MethodPost, "/api/v1/executions/"+url.PathEscape(id)+"/retry", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// PauseExecution pauses an active or waiting execution.
func (c *Client) PauseExecution(ctx context.Context, id string) (*Execution, error) {
	var res Execution
	if err := c.request(ctx, http.MethodPost, "/api/v1/executions/"+url.PathEscape(id)+"/pause", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ResumeExecution resumes a paused execution.
func (c *Client) ResumeExecution(ctx context.Context, id string) (*Execution, error) {
	var res Execution
	if err := c.request(ctx, http.MethodPost, "/api/v1/executions/"+url.PathEscape(id)+"/resume", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// CancelExecution cancels an active or paused execution.
func (c *Client) CancelExecution(ctx context.Context, id string) (*Execution, error) {
	var res Execution
	if err := c.request(ctx, http.MethodPost, "/api/v1/executions/"+url.PathEscape(id)+"/cancel", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// CompensateExecution triggers operator saga rollback for an execution.
func (c *Client) CompensateExecution(ctx context.Context, id string, reason string) (*Execution, error) {
	payload := map[string]string{"reason": reason}
	var res Execution
	if err := c.request(ctx, http.MethodPost, "/api/v1/executions/"+url.PathEscape(id)+"/compensate", nil, payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// GetExecutionLogs retrieves granular step audit logs for an execution run.
func (c *Client) GetExecutionLogs(ctx context.Context, id string) ([]ExecutionStep, error) {
	exec, err := c.GetExecution(ctx, id)
	if err != nil {
		return nil, err
	}
	return exec.Steps, nil
}

// --- Human Approval APIs ---

// ApprovalFilter defines query parameters for listing approvals.
type ApprovalFilter struct {
	WorkflowID  string
	ExecutionID string
	Status      string
	Limit       int
	Offset      int
}

// ListApprovals lists pending and historical human approvals.
func (c *Client) ListApprovals(ctx context.Context, filters ...ApprovalFilter) ([]Approval, error) {
	q := url.Values{}
	if len(filters) > 0 {
		f := filters[0]
		if f.WorkflowID != "" {
			q.Set("workflow_id", f.WorkflowID)
		}
		if f.ExecutionID != "" {
			q.Set("execution_id", f.ExecutionID)
		}
		if f.Status != "" {
			q.Set("status", f.Status)
		}
		if f.Limit > 0 {
			q.Set("limit", strconv.Itoa(f.Limit))
		}
		if f.Offset > 0 {
			q.Set("offset", strconv.Itoa(f.Offset))
		}
	}
	var res []Approval
	if err := c.request(ctx, http.MethodGet, "/api/v1/approvals", q, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetApproval retrieves approval record by ID.
func (c *Client) GetApproval(ctx context.Context, id string) (*Approval, error) {
	var res Approval
	if err := c.request(ctx, http.MethodGet, "/api/v1/approvals/"+url.PathEscape(id), nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Approve authorizes an approval step.
func (c *Client) Approve(ctx context.Context, id string, note string) (*Approval, error) {
	payload := map[string]string{"note": note}
	var res Approval
	if err := c.request(ctx, http.MethodPost, "/api/v1/approvals/"+url.PathEscape(id)+"/approve", nil, payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Reject rejects an approval step, initiating saga compensation rollback.
func (c *Client) Reject(ctx context.Context, id string, note string) (*Approval, error) {
	payload := map[string]string{"note": note}
	var res Approval
	if err := c.request(ctx, http.MethodPost, "/api/v1/approvals/"+url.PathEscape(id)+"/reject", nil, payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// GetApprovalByToken retrieves safe public preview of an approval via cryptographic token.
func (c *Client) GetApprovalByToken(ctx context.Context, token string) (map[string]any, error) {
	var res map[string]any
	if err := c.request(ctx, http.MethodGet, "/api/v1/approval-tokens/"+url.PathEscape(token), nil, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// DecideApprovalByToken applies an approval or rejection decision using cryptographic token.
func (c *Client) DecideApprovalByToken(ctx context.Context, token string, decision string, note string, approver string) (*Approval, error) {
	payload := map[string]string{
		"decision": decision,
		"note":     note,
		"approver": approver,
	}
	var res Approval
	if err := c.request(ctx, http.MethodPost, "/api/v1/approval-tokens/"+url.PathEscape(token), nil, payload, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// --- Event APIs ---

// IngestEvent submits a new event into the Kerob automation pipeline.
func (c *Client) IngestEvent(ctx context.Context, sourceID, eventType string, payload any) (*EventRecord, error) {
	pBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	body := map[string]any{
		"source_id":  sourceID,
		"event_type": eventType,
		"payload":    json.RawMessage(pBytes),
	}
	var res EventRecord
	if err := c.request(ctx, http.MethodPost, "/api/v1/events", nil, body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// GetEvent retrieves an event record by ID.
func (c *Client) GetEvent(ctx context.Context, id string) (*EventRecord, error) {
	var res EventRecord
	if err := c.request(ctx, http.MethodGet, "/api/v1/events/"+url.PathEscape(id), nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ListEvents lists recently ingested events.
func (c *Client) ListEvents(ctx context.Context, limit, offset int) ([]EventRecord, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	var res []EventRecord
	if err := c.request(ctx, http.MethodGet, "/api/v1/events", q, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// ReplayEvent triggers automation replay of a recorded event.
func (c *Client) ReplayEvent(ctx context.Context, id string, bypassIdempotency bool) (map[string]any, error) {
	body := map[string]bool{"bypass_idempotency": bypassIdempotency}
	var res map[string]any
	if err := c.request(ctx, http.MethodPost, "/api/v1/events/"+url.PathEscape(id)+"/replay", nil, body, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// --- Dead-Letter Failures (DLQ) APIs ---

// ListFailures retrieves dead-letter failure records.
func (c *Client) ListFailures(ctx context.Context, includeDiscarded bool) ([]DeadLetter, error) {
	q := url.Values{}
	if includeDiscarded {
		q.Set("include_discarded", "true")
	}
	var res []DeadLetter
	if err := c.request(ctx, http.MethodGet, "/api/v1/failures", q, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetFailure retrieves a dead-letter failure by ID.
func (c *Client) GetFailure(ctx context.Context, id string) (*DeadLetter, error) {
	var res DeadLetter
	if err := c.request(ctx, http.MethodGet, "/api/v1/failures/"+url.PathEscape(id), nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// RetryFailure retries a dead-letter failure.
func (c *Client) RetryFailure(ctx context.Context, id string) (*Execution, error) {
	var res Execution
	if err := c.request(ctx, http.MethodPost, "/api/v1/failures/"+url.PathEscape(id)+"/retry", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// DiscardFailure marks a dead-letter record discarded.
func (c *Client) DiscardFailure(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodPost, "/api/v1/failures/"+url.PathEscape(id)+"/discard", nil, nil, nil)
}

// BulkRetryFailures retries multiple dead-letter records.
func (c *Client) BulkRetryFailures(ctx context.Context, ids []string) (int, error) {
	body := map[string][]string{"ids": ids}
	var res struct {
		RetriedCount int `json:"retried_count"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/v1/failures/retry-bulk", nil, body, &res); err != nil {
		return 0, err
	}
	return res.RetriedCount, nil
}

// BulkDiscardFailures discards multiple dead-letter records.
func (c *Client) BulkDiscardFailures(ctx context.Context, ids []string) (int, error) {
	body := map[string][]string{"ids": ids}
	var res struct {
		DiscardedCount int `json:"discarded_count"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/v1/failures/discard-bulk", nil, body, &res); err != nil {
		return 0, err
	}
	return res.DiscardedCount, nil
}

// GetGovernancePolicy retrieves the active tenant governance policy.
func (c *Client) GetGovernancePolicy(ctx context.Context) (*GovernancePolicy, error) {
	var p GovernancePolicy
	if err := c.request(ctx, http.MethodGet, "/api/v1/governance/policy", nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateGovernancePolicy creates or updates the tenant governance policy.
func (c *Client) UpdateGovernancePolicy(ctx context.Context, policy *GovernancePolicy) (*GovernancePolicy, error) {
	var p GovernancePolicy
	if err := c.request(ctx, http.MethodPut, "/api/v1/governance/policy", nil, policy, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ValidateWorkflowPolicy dry-runs policy checks against candidate workflow configuration.
func (c *Client) ValidateWorkflowPolicy(ctx context.Context, wf any) (*PolicyValidationResult, error) {
	var res PolicyValidationResult
	if err := c.request(ctx, http.MethodPost, "/api/v1/governance/policy/validate", nil, wf, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// VerifyAuditChain validates the tamper-evident cryptographic hash chain of the tenant's audit trail.
func (c *Client) VerifyAuditChain(ctx context.Context) (*AuditVerificationResult, error) {
	var res AuditVerificationResult
	if err := c.request(ctx, http.MethodGet, "/api/v1/governance/audit/verify", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ExportAuditLogs downloads audit logs formatted as JSON or CSV.
func (c *Client) ExportAuditLogs(ctx context.Context, format string, filter map[string]string) ([]byte, error) {
	q := url.Values{}
	if format != "" {
		q.Set("format", format)
	}
	for k, v := range filter {
		if v != "" {
			q.Set(k, v)
		}
	}
	var raw json.RawMessage
	if err := c.request(ctx, http.MethodGet, "/api/v1/audit-logs/export", q, nil, &raw); err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

// PruneRetention triggers a cleanup of historical records exceeding the configured retention period.
func (c *Client) PruneRetention(ctx context.Context) (*RetentionPruneResult, error) {
	var res RetentionPruneResult
	if err := c.request(ctx, http.MethodPost, "/api/v1/governance/retention/prune", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// GetAIStatus fetches the controlled AI layer configuration and operational availability.
func (c *Client) GetAIStatus(ctx context.Context) (*AIStatusResponse, error) {
	var res AIStatusResponse
	if err := c.request(ctx, http.MethodGet, "/api/v1/ai/status", nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// DraftWorkflowAI proposes a declarative workflow JSON definition from natural language instructions.
func (c *Client) DraftWorkflowAI(ctx context.Context, req AIDraftWorkflowRequest) (*AIProposal, error) {
	var res AIProposal
	if err := c.request(ctx, http.MethodPost, "/api/v1/ai/draft-workflow", nil, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// MapFieldsAI proposes key-to-key payload mappings between source schemas and target destinations.
func (c *Client) MapFieldsAI(ctx context.Context, req AIMapFieldsRequest) (*AIProposal, error) {
	var res AIProposal
	if err := c.request(ctx, http.MethodPost, "/api/v1/ai/map-fields", nil, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// GenerateConditionsAI generates structured step filter conditions from user intent.
func (c *Client) GenerateConditionsAI(ctx context.Context, req AIGenerateConditionsRequest) (*AIProposal, error) {
	var res AIProposal
	if err := c.request(ctx, http.MethodPost, "/api/v1/ai/generate-conditions", nil, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// SuggestTransformationsAI suggests payload filters, transformations, or field rewrites.
func (c *Client) SuggestTransformationsAI(ctx context.Context, req AISuggestTransformationRequest) (*AIProposal, error) {
	var res AIProposal
	if err := c.request(ctx, http.MethodPost, "/api/v1/ai/suggest-transformations", nil, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ExplainFailureAI analyzes execution errors, dead letters, and trace logs to produce safe human explanations.
func (c *Client) ExplainFailureAI(ctx context.Context, req AIExplainFailureRequest) (*AIProposal, error) {
	var res AIProposal
	if err := c.request(ctx, http.MethodPost, "/api/v1/ai/explain-failure", nil, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ListAIProposals retrieves historical AI proposals for the caller's tenant.
func (c *Client) ListAIProposals(ctx context.Context, task string, status string, limit, offset int) ([]*AIProposal, error) {
	q := url.Values{}
	if task != "" {
		q.Set("task_type", task)
	}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	var res []*AIProposal
	if err := c.request(ctx, http.MethodGet, "/api/v1/ai/proposals", q, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetAIProposal retrieves a single AI proposal by ID.
func (c *Client) GetAIProposal(ctx context.Context, id string) (*AIProposal, error) {
	var res AIProposal
	if err := c.request(ctx, http.MethodGet, "/api/v1/ai/proposals/"+url.PathEscape(id), nil, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// AcceptAIProposal records explicit human approval and optionally instantiates the workflow.
func (c *Client) AcceptAIProposal(ctx context.Context, id string, createWorkflow bool) (*AIAcceptProposalResult, error) {
	q := url.Values{}
	if createWorkflow {
		q.Set("create_workflow", "true")
	}

	var res AIAcceptProposalResult
	if err := c.request(ctx, http.MethodPost, "/api/v1/ai/proposals/"+url.PathEscape(id)+"/accept", q, map[string]bool{"create_workflow": createWorkflow}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// RejectAIProposal records explicit human rejection of an AI proposal with an optional reason note.
func (c *Client) RejectAIProposal(ctx context.Context, id string, reason string) (*AIProposal, error) {
	var res AIProposal
	body := map[string]string{"reason": reason}
	if err := c.request(ctx, http.MethodPost, "/api/v1/ai/proposals/"+url.PathEscape(id)+"/reject", nil, body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
