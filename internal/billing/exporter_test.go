package billing

import (
	"context"
	"testing"
	"time"
)

func TestAggregateEvents(t *testing.T) {
	now := time.Now()

	events := []MeteringEvent{
		{
			Type:      EventSessionHour,
			UserID:    "user1",
			SessionID: "s1",
			AppID:     "app1",
			Timestamp: now,
			Quantity:  1.5,
			Unit:      "hours",
		},
		{
			Type:      EventSessionHour,
			UserID:    "user1",
			SessionID: "s2",
			AppID:     "app2",
			Timestamp: now.Add(time.Hour),
			Quantity:  0.5,
			Unit:      "hours",
		},
		{
			Type:      EventSessionHour,
			UserID:    "user2",
			SessionID: "s3",
			AppID:     "app1",
			Timestamp: now,
			Quantity:  3.0,
			Unit:      "hours",
		},
		{
			Type:      EventActiveUser,
			UserID:    "user1",
			Timestamp: now,
			Quantity:  1,
			Unit:      "active",
		},
	}

	records := AggregateEvents(events)

	// Should have 3 records: user1/session_hour, user2/session_hour, user1/active_user
	if len(records) != 3 {
		t.Fatalf("expected 3 aggregated records, got %d", len(records))
	}

	// Find user1 session_hour record
	var user1Hours *UsageRecord
	for i, r := range records {
		if r.CustomerID == "user1" && r.Dimension == "session_hour" {
			user1Hours = &records[i]
			break
		}
	}

	if user1Hours == nil {
		t.Fatal("expected user1 session_hour record")
	}

	if user1Hours.Quantity != 2.0 {
		t.Errorf("expected 2.0 aggregated hours for user1, got %f", user1Hours.Quantity)
	}
}

func TestAggregateEventsEmpty(t *testing.T) {
	records := AggregateEvents(nil)
	if len(records) != 0 {
		t.Errorf("expected 0 records for nil input, got %d", len(records))
	}
}

func TestLogExporter(t *testing.T) {
	exporter := &LogExporter{}

	if exporter.Name() != "log" {
		t.Errorf("expected name 'log', got %q", exporter.Name())
	}

	records := []UsageRecord{
		{
			CustomerID:     "user1",
			Dimension:      "session_hour",
			Quantity:        2.5,
			Timestamp:       time.Now(),
			IdempotencyKey: "key1",
		},
		{
			CustomerID:     "user2",
			Dimension:      "active_user",
			Quantity:        1,
			Timestamp:       time.Now(),
			IdempotencyKey: "key2",
		},
	}

	result, err := exporter.Export(context.Background(), records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RecordsExported != 2 {
		t.Errorf("expected 2 records exported, got %d", result.RecordsExported)
	}
	if result.RecordsFailed != 0 {
		t.Errorf("expected 0 failures, got %d", result.RecordsFailed)
	}
}

func TestWebhookExporter(t *testing.T) {
	exporter := &WebhookExporter{Endpoint: "https://api.stripe.com/v1/usage_records"}

	if exporter.Name() != "webhook" {
		t.Errorf("expected name 'webhook', got %q", exporter.Name())
	}

	records := []UsageRecord{
		{
			CustomerID: "user1",
			Dimension:  "session_hour",
			Quantity:    1.0,
			Timestamp:   time.Now(),
		},
	}

	result, err := exporter.Export(context.Background(), records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RecordsExported != 1 {
		t.Errorf("expected 1 record exported, got %d", result.RecordsExported)
	}
}

func TestUsageRecordFields(t *testing.T) {
	now := time.Now()
	rec := UsageRecord{
		CustomerID:         "cust_123",
		SubscriptionItemID: "si_456",
		Dimension:          "session_hour",
		Quantity:            3.5,
		Timestamp:           now,
		IdempotencyKey:     "idem_789",
		Metadata: map[string]string{
			"app_id": "app1",
		},
	}

	if rec.CustomerID != "cust_123" {
		t.Error("CustomerID mismatch")
	}
	if rec.SubscriptionItemID != "si_456" {
		t.Error("SubscriptionItemID mismatch")
	}
	if rec.Dimension != "session_hour" {
		t.Error("Dimension mismatch")
	}
	if rec.Quantity != 3.5 {
		t.Error("Quantity mismatch")
	}
	if rec.IdempotencyKey != "idem_789" {
		t.Error("IdempotencyKey mismatch")
	}
	if rec.Metadata["app_id"] != "app1" {
		t.Error("Metadata mismatch")
	}
}

func TestExportResultTracking(t *testing.T) {
	result := &ExportResult{
		RecordsExported: 5,
		RecordsFailed:   1,
		ExportedAt:      time.Now(),
		Errors:          []string{"record 3 failed: timeout"},
	}

	if result.RecordsExported != 5 {
		t.Error("RecordsExported mismatch")
	}
	if result.RecordsFailed != 1 {
		t.Error("RecordsFailed mismatch")
	}
	if len(result.Errors) != 1 {
		t.Error("Errors count mismatch")
	}
}
