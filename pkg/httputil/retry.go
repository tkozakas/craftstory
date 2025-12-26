package httputil

import (
	"math/rand"
	"net"
	"net/http"
	"time"
)

type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

type RetryClient struct {
	client *http.Client
	config RetryConfig
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

func NewRetryClient(client *http.Client, config RetryConfig) *RetryClient {
	if client == nil {
		client = http.DefaultClient
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.InitialDelay == 0 {
		config.InitialDelay = 500 * time.Millisecond
	}
	if config.MaxDelay == 0 {
		config.MaxDelay = 5 * time.Second
	}
	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}

	return &RetryClient{
		client: client,
		config: config,
	}
}

func (c *RetryClient) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	delay := c.config.InitialDelay

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if req.GetBody != nil {
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return nil, bodyErr
				}
				req.Body = body
			}

			time.Sleep(applyJitter(delay))
			delay = min(time.Duration(float64(delay)*c.config.Multiplier), c.config.MaxDelay)
		}

		resp, err = c.client.Do(req)
		if !shouldRetry(resp, err) {
			return resp, err
		}

		if resp != nil {
			_ = resp.Body.Close()
		}
	}

	return resp, err
}

func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true
		}
		if _, ok := err.(*net.OpError); ok {
			return true
		}
		if _, ok := err.(*net.DNSError); ok {
			return true
		}
		return false
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}

	return resp.StatusCode >= 500 && resp.StatusCode < 600
}

func applyJitter(delay time.Duration) time.Duration {
	jitterFactor := 0.9 + rand.Float64()*0.2
	return time.Duration(float64(delay) * jitterFactor)
}
