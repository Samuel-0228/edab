package transformations

import (
	"testing"

	"kerob/internal/core/event"
)

func TestTransformPick(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"id":       "user-123",
		"password": "super-secret-password",
		"email":    "user@example.com",
		"role":     "admin",
	}, nil)

	tr := &Transformation{
		Type:     "pick",
		PickKeys: []string{"id", "email"},
	}

	out, err := Transform(tr, ev)
	if err != nil {
		t.Fatalf("Transform(pick) failed: %v", err)
	}

	if len(out) != 2 {
		t.Errorf("expected 2 keys, got %d", len(out))
	}
	if out["id"] != "user-123" || out["email"] != "user@example.com" {
		t.Errorf("unexpected pick output: %v", out)
	}
	if _, hasPass := out["password"]; hasPass {
		t.Errorf("pick output must not contain password")
	}
}

func TestTransformOmit(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"id":       "user-123",
		"password": "super-secret-password",
		"token":    "secret-token-xyz",
		"email":    "user@example.com",
	}, nil)

	tr := &Transformation{
		Type:     "omit",
		OmitKeys: []string{"password", "token"},
	}

	out, err := Transform(tr, ev)
	if err != nil {
		t.Fatalf("Transform(omit) failed: %v", err)
	}

	if len(out) != 2 {
		t.Errorf("expected 2 keys, got %d", len(out))
	}
	if _, hasPass := out["password"]; hasPass {
		t.Errorf("omit output must not contain password")
	}
	if _, hasToken := out["token"]; hasToken {
		t.Errorf("omit output must not contain token")
	}
	if out["id"] != "user-123" || out["email"] != "user@example.com" {
		t.Errorf("unexpected omit output: %v", out)
	}
}

func TestTransformFlatten(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"id": "item-99",
		"meta": map[string]any{
			"owner": "alice",
			"geo": map[string]any{
				"city": "Reykjavik",
			},
		},
	}, nil)

	tr := &Transformation{
		Type: "flatten",
	}

	out, err := Transform(tr, ev)
	if err != nil {
		t.Fatalf("Transform(flatten) failed: %v", err)
	}

	if out["id"] != "item-99" {
		t.Errorf("expected id item-99, got %v", out["id"])
	}
	if out["meta.owner"] != "alice" {
		t.Errorf("expected meta.owner alice, got %v", out["meta.owner"])
	}
	if out["meta.geo.city"] != "Reykjavik" {
		t.Errorf("expected meta.geo.city Reykjavik, got %v", out["meta.geo.city"])
	}
}

func TestTransformMerge(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"service": "api-gateway",
		"status":  "ok",
	}, nil)

	tr := &Transformation{
		Type: "merge",
		Data: map[string]any{
			"environment": "production",
			"cluster":     "eu-west-1",
			"echo_svc":    "{{payload.service}}",
		},
	}

	out, err := Transform(tr, ev)
	if err != nil {
		t.Fatalf("Transform(merge) failed: %v", err)
	}

	if out["service"] != "api-gateway" {
		t.Errorf("expected service retained, got %v", out["service"])
	}
	if out["environment"] != "production" {
		t.Errorf("expected environment merged, got %v", out["environment"])
	}
	if out["echo_svc"] != "api-gateway" {
		t.Errorf("expected echo_svc interpolated from template, got %v", out["echo_svc"])
	}
}

func TestTransformCompute(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"subtotal":   100.0,
		"tax":        8.5,
		"discount":   10.0,
		"quantity":   4.0,
		"first_name": "Antigravity",
		"last_name":  "Bot",
	}, nil)

	tr := &Transformation{
		Type: "compute",
		Compute: []ComputeField{
			{
				Target: "total",
				Op:     "add",
				Args:   []string{"payload.subtotal", "payload.tax"},
			},
			{
				Target: "net_total",
				Op:     "sub",
				Args:   []string{"payload.subtotal", "payload.discount"},
			},
			{
				Target: "line_items_cost",
				Op:     "mul",
				Args:   []string{"payload.subtotal", "payload.quantity"},
			},
			{
				Target: "unit_price",
				Op:     "div",
				Args:   []string{"payload.subtotal", "payload.quantity"},
			},
			{
				Target: "full_name",
				Op:     "concat",
				Args:   []string{"payload.first_name", " ", "payload.last_name"},
			},
		},
	}

	out, err := Transform(tr, ev)
	if err != nil {
		t.Fatalf("Transform(compute) failed: %v", err)
	}

	if out["total"] != 108.5 {
		t.Errorf("expected total 108.5, got %v", out["total"])
	}
	if out["net_total"] != 90.0 {
		t.Errorf("expected net_total 90.0, got %v", out["net_total"])
	}
	if out["line_items_cost"] != 400.0 {
		t.Errorf("expected line_items_cost 400.0, got %v", out["line_items_cost"])
	}
	if out["unit_price"] != 25.0 {
		t.Errorf("expected unit_price 25.0, got %v", out["unit_price"])
	}
	if out["full_name"] != "Antigravity Bot" {
		t.Errorf("expected full_name 'Antigravity Bot', got %v", out["full_name"])
	}
}
