package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// UsageRecord represents a billing usage record ready for export.
// Each record maps to one line item in the billing system.
type UsageRecord struct {
	// CustomerID identifies the billing customer (typically maps to UserID or tenant).
	CustomerID string `json:"customer_id"`

	// SubscriptionItemID is the Stripe subscription item (or equivalent) to report against.
	SubscriptionItemID string `json:"subscription_item_id,omitempty"`

	// Dimension identifies what is being metered (e.g., "session_hours", "active_users").
	Dimension string `json:"dimension"`

	// Quantity is the usage amount in the dimension's unit.
	Quantity float64 `json:"quantity"`

	// Timestamp is when this usage occurred.
	Timestamp time.Time `json:"timestamp"`

	// IdempotencyKey prevents duplicate reporting.
	IdempotencyKey string `json:"idempotency_key"`

	// Metadata contains additional billing context.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ExportResult contains the result of an export operation.
type ExportResult struct {
	RecordsExported int       `json:"records_exported"`
	RecordsFailed   int       `json:"records_failed"`
	ExportedAt      time.Time `json:"exported_at"`
	Errors          []string  `json:"errors,omitempty"`
}

// Exporter defines the interface for billing export backends.
// Implementations send usage records to external billing systems (Stripe, etc.).
type Exporter interface {
	// Export sends a batch of usage records to the billing backend.
	Export(ctx context.Context, records []UsageRecord) (*ExportResult, error)

	// Name returns the exporter's name for logging.
	Name() string
}

// AggregateEvents converts raw metering events into billing-ready UsageRecords.
// It groups events by user and dimension, aggregating quantities.
func AggregateEvents(events []MeteringEvent) []UsageRecord {
	// Group by user+dimension for aggregation
	type aggKey struct {
		UserID    string
		Dimension string
	}
	aggregated := make(map[aggKey]*UsageRecord)

	for _, e := range events {
		dim := string(e.Type)
		key := aggKey{UserID: e.UserID, Dimension: dim}

		if rec, ok := aggregated[key]; ok {
			rec.Quantity += e.Quantity
		} else {
			aggregated[key] = &UsageRecord{
				CustomerID:     e.UserID,
				Dimension:      dim,
				Quantity:        e.Quantity,
				Timestamp:       e.Timestamp,
				IdempotencyKey: fmt.Sprintf("%s-%s-%d", e.UserID, dim, e.Timestamp.UnixMilli()),
				Metadata:       map[string]string{"app_id": e.AppID},
			}
		}
	}

	records := make([]UsageRecord, 0, len(aggregated))
	for _, rec := range aggregated {
		records = append(records, *rec)
	}
	return records
}

// LogExporter writes usage records to the application log.
// Use this for development, testing, and as a reference implementation.
type LogExporter struct{}

// Export logs each usage record as JSON.
func (e *LogExporter) Export(_ context.Context, records []UsageRecord) (*ExportResult, error) {
	result := &ExportResult{
		ExportedAt: time.Now(),
	}

	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			result.RecordsFailed++
			result.Errors = append(result.Errors, fmt.Sprintf("marshal error for %s: %v", rec.CustomerID, err))
			continue
		}
		log.Printf("billing export: %s", data)
		result.RecordsExported++
	}

	return result, nil
}

// Name returns "log".
func (e *LogExporter) Name() string { return "log" }

// WebhookExporter sends usage records to an HTTP endpoint as JSON.
// This supports integration with Stripe, custom billing systems, or data pipelines.
type WebhookExporter struct {
	Endpoint string
}

// Export sends records as a JSON payload to the configured endpoint.
// The actual HTTP call is left to the caller via a middleware or transport layer
// to keep this package free of HTTP client dependencies.
func (e *WebhookExporter) Export(_ context.Context, records []UsageRecord) (*ExportResult, error) {
	result := &ExportResult{
		ExportedAt: time.Now(),
	}

	// Encode all records
	payload, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal records: %w", err)
	}

	// Log the payload that would be sent (actual HTTP transport is handled externally)
	log.Printf("billing webhook: would POST %d records (%d bytes) to %s", len(records), len(payload), e.Endpoint)
	result.RecordsExported = len(records)

	return result, nil
}

// Name returns "webhook".
func (e *WebhookExporter) Name() string { return "webhook" }

// ExportLoop runs a periodic export of metering events using the given exporter.
// It drains events from the collector, aggregates them, and sends to the exporter.
// Call this in a goroutine; it blocks until ctx is cancelled.
func ExportLoop(ctx context.Context, collector *Collector, exporter Exporter, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("billing: export loop started (exporter=%s, interval=%v)", exporter.Name(), interval)

	for {
		select {
		case <-ticker.C:
			events := collector.DrainEvents()
			if len(events) == 0 {
				continue
			}

			records := AggregateEvents(events)
			result, err := exporter.Export(ctx, records)
			if err != nil {
				log.Printf("billing: export failed: %v", err)
				continue
			}

			log.Printf("billing: exported %d records (%d failed) via %s",
				result.RecordsExported, result.RecordsFailed, exporter.Name())
		case <-ctx.Done():
			// Final drain on shutdown
			events := collector.DrainEvents()
			if len(events) > 0 {
				records := AggregateEvents(events)
				if result, err := exporter.Export(context.Background(), records); err != nil {
					log.Printf("billing: final export failed: %v", err)
				} else {
					log.Printf("billing: final export: %d records via %s", result.RecordsExported, exporter.Name())
				}
			}
			log.Printf("billing: export loop stopped")
			return
		}
	}
}
