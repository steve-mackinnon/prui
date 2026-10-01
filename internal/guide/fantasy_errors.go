package guide

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"charm.land/fantasy"
)

// providerFailureReason returns only application-owned text. Provider messages,
// URLs, headers and request bodies must never become durable guide content.
func providerFailureReason(err error) string {
	var provider *fantasy.ProviderError
	if errors.As(err, &provider) {
		code, kind, message := providerErrorFields(provider.ResponseBody)
		for _, value := range []string{code, kind} {
			switch value {
			case "insufficient_quota", "quota_exceeded", "billing_hard_limit_reached":
				return "API quota exhausted; check billing and credits"
			case "credit_balance_too_low", "credits_expired":
				return "API credits unavailable; check billing and credit balance"
			}
		}
		// Anthropic reports depleted credits as invalid_request_error, without a
		// dedicated code. Recognize its narrow phrase, but never echo the message.
		if provider.StatusCode == http.StatusBadRequest && kind == "invalid_request_error" && strings.HasPrefix(message, "Your credit balance is too low to access the Anthropic API") {
			return "API credits unavailable; check billing and credit balance"
		}
		if provider.AuthError || provider.StatusCode == http.StatusUnauthorized || kind == "authentication_error" {
			return "API authentication failed; check or renew credentials"
		}
		if provider.ContextTooLargeErr || code == "context_length_exceeded" {
			return "guide input exceeds the model context limit; choose a larger model"
		}
		switch {
		case provider.StatusCode == http.StatusForbidden:
			return "API access denied; check model and account permissions"
		case provider.StatusCode == http.StatusNotFound:
			return "API model or endpoint not found; check guide settings"
		case provider.StatusCode == http.StatusTooManyRequests || kind == "rate_limit_error" || code == "rate_limit_exceeded":
			return "API rate limit reached; try again later"
		case provider.StatusCode == http.StatusRequestTimeout || provider.StatusCode == http.StatusGatewayTimeout:
			return "guide provider request timed out; try again later"
		case provider.StatusCode >= 500 || provider.TransientError:
			return "guide provider temporarily unavailable; try again later"
		case provider.StatusCode == http.StatusBadRequest || provider.StatusCode == http.StatusUnprocessableEntity:
			return "API rejected the guide request; check model and provider settings"
		}
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout()) {
		return "guide provider request timed out; try again later"
	}
	if errors.As(err, &network) {
		return "cannot connect to guide provider; check network and endpoint"
	}
	return "guide provider request failed"
}

func providerErrorFields(data []byte) (code, kind, message string) {
	if len(data) > fantasyResponseByteLimit {
		return
	}
	// Fantasy SDK adapters supply either JSON or a full HTTP response dump.
	if bytes.HasPrefix(data, []byte("HTTP/")) {
		response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
		if err != nil {
			return
		}
		defer response.Body.Close()
		data, err = io.ReadAll(io.LimitReader(response.Body, fantasyResponseByteLimit+1))
		if err != nil || len(data) > fantasyResponseByteLimit {
			return
		}
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return
	}
	return envelope.Error.Code, envelope.Error.Type, envelope.Error.Message
}
