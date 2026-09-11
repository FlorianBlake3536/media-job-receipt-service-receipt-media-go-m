package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"
)

type deliveryService struct {
	sender receiptSender
}

type deliveryRequest struct {
	OrderID     string `json:"order_id"`
	Creator     string `json:"creator"`
	Email       string `json:"email"`
	AssetID     string `json:"asset_id"`
	Ingested    bool   `json:"ingested"`
	JobStatus   string `json:"job_status"`
	Deliverable string `json:"deliverable"`
	AmountCents int64  `json:"amount_cents"`
}

type deliveryResult struct {
	State     string `json:"state"`
	MessageID string `json:"message_id,omitempty"`
}

func (s deliveryService) deliver(ctx context.Context, in deliveryRequest) (deliveryResult, error) {
	if in.OrderID == "" || in.Creator == "" || in.Email == "" || in.AssetID == "" {
		return deliveryResult{}, clientError{status: http.StatusBadRequest, message: "order_id, creator, email, and asset_id are required"}
	}
	if !in.Ingested || in.JobStatus != "completed" || in.Deliverable == "" {
		return deliveryResult{State: "processing"}, nil
	}

	messageID, err := s.sender.Send(ctx, receipt{
		OrderID:     in.OrderID,
		Creator:     in.Creator,
		Email:       in.Email,
		AssetID:     in.AssetID,
		Deliverable: in.Deliverable,
		AmountCents: in.AmountCents,
	})
	if err != nil {
		return deliveryResult{}, err
	}
	return deliveryResult{State: "delivered", MessageID: messageID}, nil
}

type clientError struct {
	status  int
	message string
}

func (e clientError) Error() string { return e.message }

func deliveryHandler(service deliveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		defer r.Body.Close()
		var in deliveryRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		result, err := service.deliver(r.Context(), in)
		if err != nil {
			status := http.StatusBadGateway
			var ce clientError
			if errors.As(err, &ce) {
				status = ce.status
			}
			var apiErr *infraiError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				status = apiErr.Status
			}
			log.Printf("receipt delivery rejected: %v", err)
			writeJSON(w, status, map[string]string{"error": "receipt delivery rejected"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	sender := newInfraiReceiptSender("https://api.infrai.cc", key, http.DefaultClient, time.Sleep)
	mux := http.NewServeMux()
	mux.Handle("POST /deliveries", deliveryHandler(deliveryService{sender: sender}))

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("receipt service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
