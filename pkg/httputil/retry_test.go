package httputil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryClientRetriesOn503(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryClientRetriesOn429(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryClientDoesNotRetryOn400(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}

func TestRetryClientDoesNotRetryOn401(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}

func TestRetryClientRespectsMaxRetries(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	maxRetries := 2
	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   maxRetries,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// Should be 1 initial + maxRetries attempts
	expectedAttempts := int32(1 + maxRetries)
	if atomic.LoadInt32(&attempts) != expectedAttempts {
		t.Errorf("expected %d attempts, got %d", expectedAttempts, attempts)
	}
}

func TestRetryClientUsesExponentialBackoff(t *testing.T) {
	var attempts int32
	var timestamps []time.Time

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamps = append(timestamps, time.Now())
		count := atomic.AddInt32(&attempts, 1)
		if count < 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	initialDelay := 50 * time.Millisecond
	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: initialDelay,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if len(timestamps) < 4 {
		t.Fatalf("expected at least 4 timestamps, got %d", len(timestamps))
	}

	// Check that delays increase (with tolerance for jitter)
	// First retry: ~50ms, Second retry: ~100ms, Third retry: ~200ms
	// Allow 30% tolerance for jitter and timing variations
	delays := []time.Duration{
		timestamps[1].Sub(timestamps[0]),
		timestamps[2].Sub(timestamps[1]),
		timestamps[3].Sub(timestamps[2]),
	}

	expectedDelays := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
	}

	for i, expected := range expectedDelays {
		// Allow 50% tolerance due to jitter (±10%) and system timing variations
		minDelay := time.Duration(float64(expected) * 0.5)
		maxDelay := time.Duration(float64(expected) * 1.5)
		if delays[i] < minDelay || delays[i] > maxDelay {
			t.Errorf("delay %d: expected ~%v (between %v and %v), got %v",
				i+1, expected, minDelay, maxDelay, delays[i])
		}
	}
}

func TestRetryClientWithRequestBody(t *testing.T) {
	var attempts int32
	var receivedBodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	bodyContent := "test body content"
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(bodyContent))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(bodyContent)), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify body was sent on each attempt
	for i, body := range receivedBodies {
		if body != bodyContent {
			t.Errorf("attempt %d: expected body %q, got %q", i+1, bodyContent, body)
		}
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", config.MaxRetries)
	}
	if config.InitialDelay != 500*time.Millisecond {
		t.Errorf("expected InitialDelay 500ms, got %v", config.InitialDelay)
	}
	if config.MaxDelay != 5*time.Second {
		t.Errorf("expected MaxDelay 5s, got %v", config.MaxDelay)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %f", config.Multiplier)
	}
}

func TestNewRetryClientAppliesDefaults(t *testing.T) {
	client := NewRetryClient(nil, RetryConfig{})

	if client.client != http.DefaultClient {
		t.Error("expected http.DefaultClient when nil is passed")
	}
	if client.config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", client.config.MaxRetries)
	}
	if client.config.InitialDelay != 500*time.Millisecond {
		t.Errorf("expected InitialDelay 500ms, got %v", client.config.InitialDelay)
	}
	if client.config.MaxDelay != 5*time.Second {
		t.Errorf("expected MaxDelay 5s, got %v", client.config.MaxDelay)
	}
	if client.config.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %f", client.config.Multiplier)
	}
}

func TestRetryClientRetriesOn500(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryClientRetriesOn502(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryClientDoesNotRetryOn404(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewRetryClient(server.Client(), RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}
