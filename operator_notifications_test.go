package stepanel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cyberducttape/StePanel/internal/safehttp"
)

func TestTerminalJobTransitionsCreateDurableOperatorAlertEvents(t *testing.T) {
	db, err := openControlPlaneDB(filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO jobs(id, kind, operation_key, state, owner, started_at, payload, cancel_requested, updated_at)
		VALUES ('job-1', 'site.restore', '', 'queued', 'shop', ?, '{}', 0, unixepoch())`, time.Now().UTC().UnixNano()); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		t.Helper()
		var value int
		if err := db.QueryRow(`SELECT COUNT(*) FROM operator_notification_events`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if got := count(); got != 0 {
		t.Fatalf("queued job created %d alerts, want none", got)
	}
	if _, err := db.Exec(`UPDATE jobs SET state='dead-letter', updated_at=unixepoch() WHERE id='job-1'`); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 1 {
		t.Fatalf("first dead-letter transition created %d alerts, want 1", got)
	}
	if _, err := db.Exec(`UPDATE jobs SET state='queued', updated_at=unixepoch() WHERE id='job-1'; UPDATE jobs SET state='dead-letter', updated_at=unixepoch() WHERE id='job-1'`); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 2 {
		t.Fatalf("second dead-letter transition created %d alerts, want 2", got)
	}
	var jobID, kind, owner, state string
	if err := db.QueryRow(`SELECT job_id, job_kind, owner, state FROM operator_notification_events ORDER BY created_at, id LIMIT 1`).Scan(&jobID, &kind, &owner, &state); err != nil {
		t.Fatal(err)
	}
	if jobID != "job-1" || kind != "site.restore" || owner != "shop" || state != "dead-letter" {
		t.Fatalf("captured alert = %q %q %q %q", jobID, kind, owner, state)
	}
	columns, err := db.Query(`SELECT name FROM pragma_table_info('operator_notification_events')`)
	if err != nil {
		t.Fatal(err)
	}
	defer columns.Close()
	var names []string
	for columns.Next() {
		var name string
		if err := columns.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := columns.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == "payload" || name == "job_error" || name == "error_detail" {
			t.Fatalf("operator outbox stores sensitive job data field %q: %v", name, names)
		}
	}
}

func TestOperatorNotificationOutboxCapturesExistingDeadLetters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-alerts.db")
	db, err := openControlPlaneDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TRIGGER jobs_alert_on_terminal_insert; DROP TRIGGER jobs_alert_on_terminal_transition;
		DROP TABLE operator_notification_events; DELETE FROM control_plane_migrations WHERE version=14;
		INSERT INTO jobs(id, kind, operation_key, state, owner, started_at, finished_at, payload, cancel_requested, updated_at)
		VALUES ('dead-1', 'site.delete', '', 'dead-letter', 'shop', ?, unixepoch(), '{}', 0, unixepoch())`, time.Now().UTC().UnixNano()); err != nil {
		t.Fatal(err)
	}
	// Migration from the preceding schema seeds unresolved dead-letter rows.
	if err := runControlPlaneMigrations(db, path); err != nil {
		t.Fatal(err)
	}
	var count int
	err = db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM operator_notification_events WHERE job_id='dead-1'`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration created %d events for existing dead-letter, want 1", count)
	}
}

func TestValidateOperatorAlertConfig(t *testing.T) {
	valid := Config{WebServer: "caddy", AlertWebhookURL: "https://alerts.example.test/stepanel", AlertWebhookSecret: strings.Repeat("s", 32)}
	if err := validateOperatorAlertConfig(valid); err != nil {
		t.Fatalf("valid signed webhook config rejected: %v", err)
	}
	for _, config := range []Config{
		{WebServer: "caddy", AlertWebhookURL: "http://127.0.0.1/alerts"},
		{WebServer: "caddy", AlertWebhookURL: "https://alerts.example.test/alerts"},
		{WebServer: "caddy", AlertWebhookURL: "https://alerts.example.test/alerts", AlertWebhookSecret: "short"},
		{WebServer: "caddy", AlertEmailTo: "operator@example.test"},
		{WebServer: "caddy", AlertEmailTo: "operator@example.test\r\nBcc: attacker@example.test", AlertEmailFrom: "panel@example.test", AlertSMTPHost: "smtp.example.test"},
	} {
		if err := validateOperatorAlertConfig(config); err == nil {
			t.Errorf("unsafe/incomplete alert config accepted: %+v", config)
		}
	}
	validSMTP := Config{WebServer: "caddy", AlertEmailTo: "operator@example.test", AlertEmailFrom: "panel@example.test", AlertSMTPHost: "smtp.example.test", AlertSMTPPort: 587}
	if err := validateOperatorAlertConfig(validSMTP); err != nil {
		t.Fatalf("valid STARTTLS SMTP config rejected: %v", err)
	}
	validSMTP.AlertSMTPPort = -1
	if err := validateOperatorAlertConfig(validSMTP); err == nil {
		t.Fatal("invalid SMTP port accepted")
	}
}

func TestOperatorWebhookDeliversSignedEventOverTLS(t *testing.T) {
	secret := strings.Repeat("k", 32)
	var gotID, gotSignature, gotBody string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-StePanel-Event-ID")
		gotSignature = r.Header.Get("X-StePanel-Signature")
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	cfg := Config{AlertWebhookURL: server.URL, AlertWebhookSecret: secret}
	policy := safehttp.Policy{AllowPrefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}
	body := []byte(`{"event":"stepanel.operator-alert.v1","state":"dead-letter"}`)
	if err := deliverOperatorWebhookWithClient(t.Context(), cfg, "event-123", body, policy, server.Client()); err != nil {
		t.Fatalf("deliver operator webhook: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotID != "event-123" || !hmac.Equal([]byte(gotSignature), []byte(want)) || gotBody != string(body) {
		t.Fatalf("webhook id/signature/body = %q %q %q", gotID, gotSignature, gotBody)
	}
}

func TestOperatorNotificationDeliveryFailuresAreDurableAndRedacted(t *testing.T) {
	db, err := openControlPlaneDB(filepath.Join(t.TempDir(), "alert-delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := newOperatorNotificationStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO operator_notification_events(id, job_id, job_kind, owner, state, created_at, webhook_next_attempt_at, email_next_attempt_at)
		VALUES ('event-1','job-1','site.restore','shop','failed',unixepoch(),unixepoch(),unixepoch())`); err != nil {
		t.Fatal(err)
	}
	cfg := Config{AlertWebhookURL: "https://alerts.example.test/hook"}
	status, err := store.status(t.Context(), cfg)
	if err != nil || status.Pending != 1 || status.FailedDeliveries != 0 {
		t.Fatalf("initial notification status = %+v, err=%v", status, err)
	}
	if err := store.recordFailure(t.Context(), "event-1", "webhook", 0, errors.New("secret=must-not-be-persisted")); err != nil {
		t.Fatal(err)
	}
	var lastError string
	if err := db.QueryRow(`SELECT last_webhook_error FROM operator_notification_events WHERE id='event-1'`).Scan(&lastError); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lastError, "must-not-be-persisted") {
		t.Fatalf("notification outbox retained raw delivery error: %q", lastError)
	}
	var failedAt sql.NullInt64
	if err := db.QueryRow(`SELECT last_webhook_failed_at FROM operator_notification_events WHERE id='event-1'`).Scan(&failedAt); err != nil {
		t.Fatal(err)
	}
	if !failedAt.Valid || time.Since(time.Unix(failedAt.Int64, 0)) > time.Minute {
		t.Fatalf("delivery failure timestamp was not recorded accurately: %+v", failedAt)
	}
	status, err = store.status(t.Context(), cfg)
	if err != nil || status.Pending != 1 || status.FailedDeliveries != 1 || status.LastFailureAt == nil {
		t.Fatalf("failed notification status = %+v, err=%v", status, err)
	}
	if err := store.markDelivered(t.Context(), "event-1", "webhook"); err != nil {
		t.Fatal(err)
	}
	status, err = store.status(t.Context(), cfg)
	if err != nil || status.Pending != 0 || status.FailedDeliveries != 0 {
		t.Fatalf("delivered notification status = %+v, err=%v", status, err)
	}
}

func TestOperatorNotificationChannelsUseIndependentRetryDeadlines(t *testing.T) {
	db, err := openControlPlaneDB(filepath.Join(t.TempDir(), "alert-retries.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := newOperatorNotificationStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO operator_notification_events(id, job_id, job_kind, owner, state, created_at, webhook_next_attempt_at, email_next_attempt_at)
		VALUES ('event-1','job-1','site.restore','shop','failed',unixepoch(),unixepoch(),unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := store.recordFailure(t.Context(), "event-1", "webhook", 0, errors.New("timeout")); err != nil {
		t.Fatal(err)
	}
	if err := store.markDelivered(t.Context(), "event-1", "email"); err != nil {
		t.Fatal(err)
	}
	items, err := store.pending(t.Context(), Config{AlertWebhookURL: "https://alerts.example.test/hook", AlertSMTPHost: "smtp.example.test"}, operatorNotificationBatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("webhook retry became due immediately after email delivery: %+v", items)
	}
	var webhookNext, emailDelivered sql.NullInt64
	if err := db.QueryRow(`SELECT webhook_next_attempt_at, email_delivered_at FROM operator_notification_events WHERE id='event-1'`).Scan(&webhookNext, &emailDelivered); err != nil {
		t.Fatal(err)
	}
	if !webhookNext.Valid || !emailDelivered.Valid || webhookNext.Int64 <= time.Now().UTC().Unix() {
		t.Fatalf("independent retry state incorrect: webhook next=%+v email delivered=%+v", webhookNext, emailDelivered)
	}
}
