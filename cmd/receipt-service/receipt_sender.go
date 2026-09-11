package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// POST /v1/email/send is the external write boundary for completed orders.
const emailSendPath = "/v1/email/send"

type receipt struct {
	OrderID     string
	Creator     string
	Email       string
	AssetID     string
	Deliverable string
	AmountCents int64
}

type receiptSender interface {
	Send(context.Context, receipt) (string, error)
}

type infraiReceiptSender struct {
	baseURL string
	key     string
	client  *http.Client
	sleep   func(time.Duration)
}

func newInfraiReceiptSender(baseURL, key string, client *http.Client, sleep func(time.Duration)) *infraiReceiptSender {
	return &infraiReceiptSender{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		client:  client,
		sleep:   sleep,
	}
}

type sendEmailBody struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type emailEnvelope struct {
	OK       bool            `json:"ok"`
	Data     emailSendData   `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type emailSendData struct {
	MessageID string `json:"message_id"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type infraiError struct {
	Status  int
	Code    string
	Message string
}

func (e *infraiError) Error() string {
	return fmt.Sprintf("infrai email.send: status=%d code=%s message=%s", e.Status, e.Code, e.Message)
}

func (s *infraiReceiptSender) Send(ctx context.Context, r receipt) (string, error) {
	payload := sendEmailBody{
		To:      r.Email,
		Subject: "Receipt for media order " + r.OrderID,
		HTML: fmt.Sprintf(
			"<h1>Order %s is ready</h1><p>%s, your processed asset %s is ready for delivery.</p><p>Deliverable: %s</p><p>Total: $%.2f</p>",
			html.EscapeString(r.OrderID), html.EscapeString(r.Creator), html.EscapeString(r.AssetID),
			html.EscapeString(r.Deliverable), float64(r.AmountCents)/100,
		),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode receipt: %w", err)
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+emailSendPath, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("build email request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+s.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "media-receipt:"+r.OrderID)

		res, err := s.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("send email request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read email response: %w", readErr)
		}

		var env emailEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return "", fmt.Errorf("decode email envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				s.sleep(retryDelay(res.Header.Get("Retry-After"), attempt))
				continue
			}
			apiErr := &infraiError{Status: res.StatusCode}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			return "", apiErr
		}
		if res.StatusCode >= 500 {
			return "", fmt.Errorf("infrai email.send transport status %d", res.StatusCode)
		}
		if env.Data.MessageID == "" {
			return "", fmt.Errorf("infrai email.send response omitted message_id")
		}
		return env.Data.MessageID, nil
	}
	return "", fmt.Errorf("infrai email.send retry limit reached")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}
