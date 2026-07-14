package empty_response_compensation_setting

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type Setting struct {
	Enabled                bool           `json:"enabled"`
	ModelRatios            map[string]int `json:"model_ratios"`
	MinQualificationAmount int            `json:"min_qualification_amount"`
	InputTokenThreshold    int            `json:"input_token_threshold"`
	OutputTokenThreshold   int            `json:"output_token_threshold"`
	ClaimWindowDays        int            `json:"claim_window_days"`
	DailyClaimLimit        int            `json:"daily_claim_limit"`
	OverclockWindowMinutes int            `json:"overclock_window_minutes"`
	OverclockEmptyCount    int            `json:"overclock_empty_count"`
	Announcement           string         `json:"announcement"`
}

var emptyResponseCompensationSetting = Setting{
	Enabled:                false,
	ModelRatios:            map[string]int{},
	MinQualificationAmount: 50,
	InputTokenThreshold:    50,
	OutputTokenThreshold:   9,
	ClaimWindowDays:        7,
	DailyClaimLimit:        0,
	OverclockWindowMinutes: 10,
	OverclockEmptyCount:    3,
	Announcement:           "",
}

var settingSnapshot atomic.Value

func init() {
	RefreshSnapshot()
	config.GlobalConfig.Register("empty_response_compensation_setting", &emptyResponseCompensationSetting)
}

func cloneSetting(source Setting) Setting {
	clone := source
	clone.ModelRatios = make(map[string]int, len(source.ModelRatios))
	for modelName, ratio := range source.ModelRatios {
		clone.ModelRatios[modelName] = ratio
	}
	return clone
}

func RefreshSnapshot() {
	settingSnapshot.Store(cloneSetting(emptyResponseCompensationSetting))
}

func Get() Setting {
	return cloneSetting(settingSnapshot.Load().(Setting))
}

func GetModelRatio(modelName string) (int, bool) {
	setting := settingSnapshot.Load().(Setting)
	ratio, ok := setting.ModelRatios[modelName]
	return ratio, ok
}

func ValidateModelRatiosJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var ratios map[string]int
	if err := common.UnmarshalJsonStr(raw, &ratios); err != nil {
		return fmt.Errorf("invalid model compensation ratios: %w", err)
	}
	for modelName, ratio := range ratios {
		if strings.TrimSpace(modelName) == "" {
			return fmt.Errorf("model name cannot be empty")
		}
		if ratio < 1 || ratio > 100 {
			return fmt.Errorf("compensation ratio for model %s must be between 1 and 100", modelName)
		}
	}
	return nil
}

func ValidateOption(key string, value string) error {
	switch key {
	case "model_ratios":
		return ValidateModelRatiosJSON(value)
	case "enabled":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("enabled must be true or false")
		}
	case "min_qualification_amount", "input_token_threshold", "output_token_threshold", "daily_claim_limit", "overclock_window_minutes", "overclock_empty_count":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return fmt.Errorf("%s must be a non-negative integer", key)
		}
	case "claim_window_days":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return fmt.Errorf("claim_window_days must be at least 1")
		}
	}
	return nil
}
