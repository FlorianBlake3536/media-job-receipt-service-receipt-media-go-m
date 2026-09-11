package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type recordingSender struct {
	calls int
	last  receipt
}

func (s *recordingSender) Send(_ context.Context, r receipt) (string, error) {
	s.calls++
	s.last = r
	return "msg_42", nil
}

func TestDeliveryDecision(t *testing.T) {
	tests := []struct {
		name      string
		request   deliveryRequest
		wantState string
		wantCalls int
	}{
		{
			name: "processing job does not send",
			request: deliveryRequest{OrderID: "ord-42", Creator: "Mina", Email: "mina@example.com", AssetID: "asset-9",
				Ingested: true, JobStatus: "processing", AmountCents: 2400},
			wantState: "processing",
			wantCalls: 0,
		},
		{
			name: "completed job sends receipt and delivery",
			request: deliveryRequest{OrderID: "ord-42", Creator: "Mina", Email: "mina@example.com", AssetID: "asset-9",
				Ingested: true, JobStatus: "completed", Deliverable: "s3://creator-delivery/asset-9/master.mp4", AmountCents: 2400},
			wantState: "delivered",
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &recordingSender{}
			got, err := (deliveryService{sender: sender}).deliver(context.Background(), tt.request)
			if err != nil {
				t.Fatalf("deliver() error = %v", err)
			}
			if got.State != tt.wantState || sender.calls != tt.wantCalls {
				t.Fatalf("deliver() state=%q calls=%d, want state=%q calls=%d", got.State, sender.calls, tt.wantState, tt.wantCalls)
			}
			if tt.wantCalls == 1 && sender.last.OrderID != tt.request.OrderID {
				t.Fatalf("sent order %q, want %q", sender.last.OrderID, tt.request.OrderID)
			}
		})
	}
}

func TestInfraiSenderRetriesRateLimit(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != emailSendPath {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") != "media-receipt:ord-42" {
			t.Fatal("missing auth or idempotency header")
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]string{"code": "RATE_LIMITED", "message": "retry later"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]string{"message_id": "msg_42"}, "metadata": map[string]any{}})
	}))
	defer server.Close()

	sender := newInfraiReceiptSender(server.URL, "test-key", server.Client(), func(time.Duration) {})
	messageID, err := sender.Send(context.Background(), receipt{
		OrderID: "ord-42", Creator: "Mina", Email: "mina@example.com", AssetID: "asset-9", Deliverable: "master.mp4", AmountCents: 2400,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if messageID != "msg_42" || calls != 2 {
		t.Fatalf("Send() messageID=%q calls=%d", messageID, calls)
	}
}
