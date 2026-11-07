package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

// FetchModels exposes only the selected desktop agent's chat catalog.
func (Provider) FetchModels(ctx context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	base := testBaseURL
	if base == "" {
		if err := ValidateCredential(cred); err != nil {
			return nil, err
		}
		base = DefaultBaseURL
	}
	raw, err := get(ctx, base+"/v3/config", headersFor(cred))
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil {
		return nil, fmt.Errorf("invalid WorkBuddy config")
	}
	if code := number(envelope["code"]); code != 0 {
		return nil, fmt.Errorf("WorkBuddy config code %d", code)
	}
	data, err := json.Marshal(unwrapData(envelope))
	if err != nil {
		return nil, err
	}
	var config struct {
		Models []struct {
			ID                string   `json:"id"`
			Name              string   `json:"name"`
			SupportsImages    bool     `json:"supportsImages"`
			SupportsReasoning bool     `json:"supportsReasoning"`
			Tags              []string `json:"tags"`
			Credits           any      `json:"credits"`
		} `json:"models"`
		Agents []struct {
			Name   string   `json:"name"`
			Tags   []string `json:"tags"`
			Models []string `json:"models"`
		} `json:"agents"`
		ModelPromotions []workBuddyPromotion `json:"modelPromotions"`
	}
	if json.Unmarshal(data, &config) != nil {
		return nil, fmt.Errorf("invalid WorkBuddy model config")
	}
	promotions := promotionFactors(config.ModelPromotions, time.Now())
	selected := -1
	for i, a := range config.Agents {
		for _, tag := range a.Tags {
			if tag == "default" {
				selected = i
				break
			}
		}
		if selected >= 0 {
			break
		}
	}
	if selected < 0 {
		for i, a := range config.Agents {
			if a.Name == "cli" {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		for i, a := range config.Agents {
			if len(a.Models) > 0 {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		return nil, fmt.Errorf("WorkBuddy config has no client model list")
	}
	out := []providers.ModelInfo{}
	seen := map[string]bool{}
	for _, id := range config.Agents[selected].Models {
		for _, m := range config.Models {
			if m.ID != id && m.Name != id {
				continue
			}
			image := false
			for _, tag := range m.Tags {
				if tag == "text-to-image" || tag == "image-to-image" {
					image = true
				}
			}
			if m.ID == "" || seen[m.ID] || image {
				continue
			}
			seen[m.ID] = true
			name := m.Name
			if name == "" {
				name = m.ID
			}
			price, isFree := modelPricing(m.Credits, promotions[m.ID], promotions[m.Name])
			out = append(out, providers.ModelInfo{
				ID:          m.ID,
				DisplayName: name,
				Multimodal:  m.SupportsImages,
				Reasoning:   m.SupportsReasoning,
				PriceFactor: price,
				IsFree:      isFree,
			})
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("WorkBuddy client model list is empty")
	}
	return out, nil
}

type workBuddyPromotion struct {
	ModelIDs []string `json:"modelIds"`
	Enabled  *bool    `json:"enabled"`
	Priority int      `json:"priority"`
	Discount struct {
		Factor any `json:"factor"`
	} `json:"discount"`
	Schedule struct {
		Timezone   string `json:"timezone"`
		ValidFrom  string `json:"validFrom"`
		ValidUntil string `json:"validUntil"`
		Daily      []struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"daily"`
	} `json:"schedule"`
}

func promotionFactors(promotions []workBuddyPromotion, now time.Time) map[string]*float64 {
	out := map[string]*float64{}
	rank := map[string]int{}
	for _, promotion := range promotions {
		if !promotionActive(promotion, now) {
			continue
		}
		factor := providers.ParsePriceFactor(promotion.Discount.Factor)
		if factor == nil {
			continue
		}
		for _, id := range promotion.ModelIDs {
			if id = strings.TrimSpace(id); id != "" {
				if current, ok := rank[id]; !ok || promotion.Priority >= current {
					out[id] = factor
					rank[id] = promotion.Priority
				}
			}
		}
	}
	return out
}

func promotionActive(promotion workBuddyPromotion, now time.Time) bool {
	if promotion.Enabled != nil && !*promotion.Enabled {
		return false
	}
	location := time.Local
	if promotion.Schedule.Timezone != "" {
		loaded, err := time.LoadLocation(promotion.Schedule.Timezone)
		if err != nil {
			return false
		}
		location = loaded
	}
	local := now.In(location)
	if promotion.Schedule.ValidFrom != "" {
		start, err := time.Parse(time.RFC3339, promotion.Schedule.ValidFrom)
		if err != nil || local.Before(start.In(location)) {
			return false
		}
	}
	if promotion.Schedule.ValidUntil != "" {
		end, err := time.Parse(time.RFC3339, promotion.Schedule.ValidUntil)
		if err != nil || !local.Before(end.In(location)) {
			return false
		}
	}
	if len(promotion.Schedule.Daily) == 0 {
		return true
	}
	minute := local.Hour()*60 + local.Minute()
	for _, window := range promotion.Schedule.Daily {
		start, startOK := parseClockMinute(window.Start)
		end, endOK := parseClockMinute(window.End)
		if !startOK || !endOK {
			continue
		}
		if start <= end {
			if minute >= start && minute < end {
				return true
			}
			continue
		}
		if minute >= start || minute < end {
			return true
		}
	}
	return false
}

func parseClockMinute(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func modelPricing(base any, promotion ...*float64) (*float64, *bool) {
	price := parseWorkBuddyCredits(base)
	for _, candidate := range promotion {
		if candidate != nil {
			price = candidate
			break
		}
	}
	var free *bool
	if price != nil && *price == 0 {
		value := true
		free = &value
	}
	return price, free
}

func parseWorkBuddyCredits(value any) *float64 {
	text, ok := value.(string)
	if !ok {
		return providers.ParsePriceFactor(value)
	}
	text = strings.TrimSpace(text)
	if len(text) > 1 && (text[0] == 'x' || text[0] == 'X') {
		text = text[1:]
	}
	text = strings.TrimSuffix(strings.TrimSpace(text), "x")
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil
	}
	return providers.ParsePriceFactor(parsed)
}
