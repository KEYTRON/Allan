package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// Balance is what a provider reports as left on the account behind a key.
type Balance struct {
	Amount   float64
	Currency string
	Note     string
}

// BalanceSupported lists providers that report a balance by API key; the
// rest (OpenAI, Anthropic, xAI…) only show it in their dashboards.
func BalanceSupported(provider string) bool {
	switch NormalizeProvider(provider) {
	case "deepseek", "openrouter":
		return true
	}
	return false
}

// ProviderBalance asks the provider how much is left. A nil Balance with a
// nil error means the provider does not report one.
func ProviderBalance(ctx context.Context, provider, apiKey string) (*Balance, error) {
	switch NormalizeProvider(provider) {
	case "deepseek":
		var r struct {
			IsAvailable  bool `json:"is_available"`
			BalanceInfos []struct {
				Currency     string `json:"currency"`
				TotalBalance string `json:"total_balance"`
			} `json:"balance_infos"`
		}
		if err := getJSON(ctx, "https://api.deepseek.com/user/balance", apiKey, &r); err != nil {
			return nil, err
		}
		for _, b := range r.BalanceInfos {
			v, _ := strconv.ParseFloat(b.TotalBalance, 64)
			out := &Balance{Amount: v, Currency: b.Currency}
			if !r.IsAvailable {
				out.Note = "нет средств"
			}
			return out, nil
		}
		return nil, nil
	case "openrouter":
		var r struct {
			Data struct {
				TotalCredits float64 `json:"total_credits"`
				TotalUsage   float64 `json:"total_usage"`
			} `json:"data"`
		}
		if err := getJSON(ctx, "https://openrouter.ai/api/v1/credits", apiKey, &r); err == nil {
			return &Balance{Amount: r.Data.TotalCredits - r.Data.TotalUsage, Currency: "USD"}, nil
		}
		// a key without access to account credits still knows its own limit
		var k struct {
			Data struct {
				LimitRemaining *float64 `json:"limit_remaining"`
			} `json:"data"`
		}
		if err := getJSON(ctx, "https://openrouter.ai/api/v1/key", apiKey, &k); err != nil {
			return nil, err
		}
		if k.Data.LimitRemaining == nil {
			return nil, nil
		}
		return &Balance{Amount: *k.Data.LimitRemaining, Currency: "USD", Note: "лимит ключа"}, nil
	}
	return nil, nil
}

func getJSON(ctx context.Context, url, apiKey string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
