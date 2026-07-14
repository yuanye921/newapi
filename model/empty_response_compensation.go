package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	emptysetting "github.com/QuantumNous/new-api/setting/empty_response_compensation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	EmptyResponseCompensationStatusPending = "pending"
	EmptyResponseCompensationStatusClaimed = "claimed"
	EmptyResponseCompensationStatusBlocked = "blocked"
	EmptyResponseCompensationStatusExpired = "expired"
)

const (
	EmptyResponseCompensationBlockHighFrequency = "high_frequency"
)

type EmptyResponseCompensation struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id" gorm:"index;uniqueIndex:idx_empty_comp_user_request,priority:1"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);uniqueIndex:idx_empty_comp_user_request,priority:2"`
	ModelName         string `json:"model_name" gorm:"type:varchar(255);index"`
	ChannelId         int    `json:"channel_id" gorm:"index"`
	TokenId           int    `json:"token_id" gorm:"index"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	OriginalQuota     int    `json:"original_quota"`
	CompensationRatio int    `json:"compensation_ratio"`
	CompensationQuota int    `json:"compensation_quota"`
	Status            string `json:"status" gorm:"type:varchar(16);index"`
	BlockReason       string `json:"block_reason" gorm:"type:varchar(64);default:''"`
	DetectionReason   string `json:"detection_reason" gorm:"type:varchar(255);default:''"`
	IsStream          bool   `json:"is_stream"`
	CreatedAt         int64  `json:"created_at" gorm:"index"`
	ExpiresAt         int64  `json:"expires_at" gorm:"index"`
	ClaimedAt         int64  `json:"claimed_at" gorm:"index"`
	Username          string `json:"username,omitempty" gorm:"->;-:migration"`
}

type CreateEmptyResponseCompensationParams struct {
	UserId            int
	RequestId         string
	ModelName         string
	ChannelId         int
	TokenId           int
	PromptTokens      int
	CompletionTokens  int
	OriginalQuota     int
	CompensationRatio int
	IsStream          bool
	DetectionReason   string
	Setting           emptysetting.Setting
}

type EmptyResponseCompensationSummary struct {
	QualificationAmount float64 `json:"qualification_amount"`
	Qualified           bool    `json:"qualified"`
	PendingCount        int64   `json:"pending_count"`
	PendingQuota        int64   `json:"pending_quota"`
	ClaimedCount        int64   `json:"claimed_count"`
	ClaimedQuota        int64   `json:"claimed_quota"`
	ClaimedToday        int64   `json:"claimed_today"`
	DailyRemaining      *int64  `json:"daily_remaining"`
}

type EmptyResponseCompensationClaimResult struct {
	ClaimedIds    []int          `json:"claimed_ids"`
	Skipped       map[int]string `json:"skipped"`
	CreditedQuota int            `json:"credited_quota"`
	ClaimedCount  int            `json:"claimed_count"`
}

type EmptyResponseCompensationAdminSummary struct {
	TotalCount   int64 `json:"total_count"`
	PendingCount int64 `json:"pending_count"`
	ClaimedCount int64 `json:"claimed_count"`
	BlockedCount int64 `json:"blocked_count"`
	ExpiredCount int64 `json:"expired_count"`
	PendingQuota int64 `json:"pending_quota"`
	ClaimedQuota int64 `json:"claimed_quota"`
}

type EmptyResponseCompensationAdminFilter struct {
	UserId    int
	ModelName string
	Status    string
	StartTime int64
	EndTime   int64
	Offset    int
	Limit     int
}

func compensationQuota(originalQuota int, ratio int) int {
	if originalQuota <= 0 || ratio <= 0 {
		return 0
	}
	value := decimal.NewFromInt(int64(originalQuota)).Mul(decimal.NewFromInt(int64(ratio))).Div(decimal.NewFromInt(100)).Round(0)
	return int(value.IntPart())
}

func localDayRange(now time.Time) (int64, int64) {
	localNow := now.In(time.Local)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	return start.Unix(), start.AddDate(0, 0, 1).Unix()
}

func CreateEmptyResponseCompensation(params CreateEmptyResponseCompensationParams) (*EmptyResponseCompensation, error) {
	if params.UserId <= 0 || params.RequestId == "" || params.ModelName == "" {
		return nil, errors.New("invalid empty response compensation parameters")
	}
	quota := compensationQuota(params.OriginalQuota, params.CompensationRatio)
	if quota <= 0 {
		return nil, nil
	}

	now := common.GetTimestamp()
	record := &EmptyResponseCompensation{
		UserId:            params.UserId,
		RequestId:         params.RequestId,
		ModelName:         params.ModelName,
		ChannelId:         params.ChannelId,
		TokenId:           params.TokenId,
		PromptTokens:      params.PromptTokens,
		CompletionTokens:  params.CompletionTokens,
		OriginalQuota:     params.OriginalQuota,
		CompensationRatio: params.CompensationRatio,
		CompensationQuota: quota,
		Status:            EmptyResponseCompensationStatusPending,
		DetectionReason:   params.DetectionReason,
		IsStream:          params.IsStream,
		CreatedAt:         now,
		ExpiresAt:         time.Unix(now, 0).In(time.Local).AddDate(0, 0, params.Setting.ClaimWindowDays).Unix(),
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		// Serialize decisions for one user before checking duplicates or the
		// overclock window. This keeps concurrent nodes on the same boundary.
		if err := tx.Model(&User{}).Where("id = ?", params.UserId).
			UpdateColumn("quota", gorm.Expr("quota")).Error; err != nil {
			return err
		}

		var duplicateCount int64
		if err := tx.Model(&EmptyResponseCompensation{}).
			Where("user_id = ? AND request_id = ?", params.UserId, params.RequestId).
			Count(&duplicateCount).Error; err != nil {
			return err
		}
		if duplicateCount > 0 {
			return nil
		}

		if params.Setting.OverclockWindowMinutes > 0 && params.Setting.OverclockEmptyCount > 0 {
			windowStart := now - int64(params.Setting.OverclockWindowMinutes*60)
			var recentCount int64
			if err := tx.Model(&EmptyResponseCompensation{}).
				Where("user_id = ? AND created_at >= ?", params.UserId, windowStart).
				Count(&recentCount).Error; err != nil {
				return err
			}
			if recentCount >= int64(params.Setting.OverclockEmptyCount) {
				record.Status = EmptyResponseCompensationStatusBlocked
				record.BlockReason = EmptyResponseCompensationBlockHighFrequency
			}
		}

		return tx.Create(record).Error
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, nil
		}
		return nil, err
	}
	if record.Id == 0 {
		return nil, nil
	}
	return record, nil
}

func expireEmptyResponseCompensations(tx *gorm.DB, userId int, now int64) error {
	query := tx.Model(&EmptyResponseCompensation{}).
		Where("status = ? AND expires_at <= ?", EmptyResponseCompensationStatusPending, now)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	return query.Update("status", EmptyResponseCompensationStatusExpired).Error
}

func GetUserEmptyResponseCompensations(userId int, offset int, limit int) ([]*EmptyResponseCompensation, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	now := common.GetTimestamp()
	if err := expireEmptyResponseCompensations(DB, userId, now); err != nil {
		return nil, 0, err
	}
	query := DB.Model(&EmptyResponseCompensation{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []*EmptyResponseCompensation
	if err := query.Order("created_at desc, id desc").Offset(offset).Limit(limit).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func getQualificationAmount(tx *gorm.DB, userId int) (float64, error) {
	var topupAmount int64
	if err := tx.Model(&TopUp{}).
		Where("user_id = ? AND status = ? AND (payment_provider IS NULL OR payment_provider <> ?)", userId, common.TopUpStatusSuccess, PaymentProviderBalance).
		Select("COALESCE(SUM(amount), 0)").Scan(&topupAmount).Error; err != nil {
		return 0, err
	}
	var redemptionQuota int64
	if err := tx.Model(&Redemption{}).
		Where("used_user_id = ? AND status = ?", userId, common.RedemptionCodeStatusUsed).
		Select("COALESCE(SUM(quota), 0)").Scan(&redemptionQuota).Error; err != nil {
		return 0, err
	}
	return float64(topupAmount) + float64(redemptionQuota)/common.QuotaPerUnit, nil
}

func GetEmptyResponseCompensationSummary(userId int, setting emptysetting.Setting) (*EmptyResponseCompensationSummary, error) {
	now := common.GetTimestamp()
	if err := expireEmptyResponseCompensations(DB, userId, now); err != nil {
		return nil, err
	}
	qualification, err := getQualificationAmount(DB, userId)
	if err != nil {
		return nil, err
	}
	summary := &EmptyResponseCompensationSummary{
		QualificationAmount: qualification,
		Qualified:           qualification >= float64(setting.MinQualificationAmount),
	}
	if err := DB.Model(&EmptyResponseCompensation{}).Where("user_id = ? AND status = ?", userId, EmptyResponseCompensationStatusPending).
		Select("COUNT(*) AS pending_count, COALESCE(SUM(compensation_quota), 0) AS pending_quota").Scan(summary).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&EmptyResponseCompensation{}).Where("user_id = ? AND status = ?", userId, EmptyResponseCompensationStatusClaimed).
		Select("COUNT(*) AS claimed_count, COALESCE(SUM(compensation_quota), 0) AS claimed_quota").Scan(summary).Error; err != nil {
		return nil, err
	}
	dayStart, dayEnd := localDayRange(time.Now())
	if err := DB.Model(&EmptyResponseCompensation{}).
		Where("user_id = ? AND status = ? AND claimed_at >= ? AND claimed_at < ?", userId, EmptyResponseCompensationStatusClaimed, dayStart, dayEnd).
		Count(&summary.ClaimedToday).Error; err != nil {
		return nil, err
	}
	if setting.DailyClaimLimit > 0 {
		remaining := int64(setting.DailyClaimLimit) - summary.ClaimedToday
		if remaining < 0 {
			remaining = 0
		}
		summary.DailyRemaining = &remaining
	}
	return summary, nil
}

func ClaimEmptyResponseCompensations(userId int, ids []int, setting emptysetting.Setting) (*EmptyResponseCompensationClaimResult, error) {
	result := &EmptyResponseCompensationClaimResult{
		ClaimedIds: []int{},
		Skipped:    map[int]string{},
	}
	if len(ids) == 0 || len(ids) > 100 {
		return nil, errors.New("claim must contain between 1 and 100 records")
	}
	uniqueIds := make([]int, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIds = append(uniqueIds, id)
	}
	if len(uniqueIds) == 0 {
		return nil, errors.New("claim contains no valid record ids")
	}

	now := common.GetTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", userId).
			UpdateColumn("quota", gorm.Expr("quota")).Error; err != nil {
			return err
		}
		if err := expireEmptyResponseCompensations(tx, userId, now); err != nil {
			return err
		}
		qualification, err := getQualificationAmount(tx, userId)
		if err != nil {
			return err
		}

		var records []EmptyResponseCompensation
		if err := tx.Where("user_id = ? AND id IN ?", userId, uniqueIds).
			Order("created_at asc, id asc").Find(&records).Error; err != nil {
			return err
		}
		byId := make(map[int]EmptyResponseCompensation, len(records))
		for _, record := range records {
			byId[record.Id] = record
		}
		for _, id := range uniqueIds {
			if _, exists := byId[id]; !exists {
				result.Skipped[id] = "not_found"
			}
		}
		if qualification < float64(setting.MinQualificationAmount) {
			for _, record := range records {
				result.Skipped[record.Id] = "not_qualified"
			}
			return nil
		}

		remaining := len(records)
		if setting.DailyClaimLimit > 0 {
			dayStart, dayEnd := localDayRange(time.Now())
			var claimedToday int64
			if err := tx.Model(&EmptyResponseCompensation{}).
				Where("user_id = ? AND status = ? AND claimed_at >= ? AND claimed_at < ?", userId, EmptyResponseCompensationStatusClaimed, dayStart, dayEnd).
				Count(&claimedToday).Error; err != nil {
				return err
			}
			remaining = setting.DailyClaimLimit - int(claimedToday)
			if remaining < 0 {
				remaining = 0
			}
		}

		for _, record := range records {
			if record.Status != EmptyResponseCompensationStatusPending {
				result.Skipped[record.Id] = record.Status
				continue
			}
			if record.ExpiresAt <= now {
				result.Skipped[record.Id] = EmptyResponseCompensationStatusExpired
				continue
			}
			if remaining <= 0 {
				result.Skipped[record.Id] = "daily_limit"
				continue
			}
			update := tx.Model(&EmptyResponseCompensation{}).
				Where("id = ? AND user_id = ? AND status = ? AND expires_at > ?", record.Id, userId, EmptyResponseCompensationStatusPending, now).
				Updates(map[string]interface{}{"status": EmptyResponseCompensationStatusClaimed, "claimed_at": now})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected == 0 {
				result.Skipped[record.Id] = "already_processed"
				continue
			}
			result.ClaimedIds = append(result.ClaimedIds, record.Id)
			result.CreditedQuota += record.CompensationQuota
			remaining--
		}

		if result.CreditedQuota > 0 {
			if err := tx.Model(&User{}).Where("id = ?", userId).
				UpdateColumn("quota", gorm.Expr("quota + ?", result.CreditedQuota)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.ClaimedCount = len(result.ClaimedIds)
	if result.CreditedQuota > 0 {
		_ = InvalidateUserCache(userId)
		RecordTaskBillingLog(RecordTaskBillingLogParams{
			UserId:  userId,
			LogType: LogTypeRefund,
			Content: fmt.Sprintf("Empty response compensation claimed: %d record(s)", result.ClaimedCount),
			Quota:   result.CreditedQuota,
			Other: map[string]interface{}{
				"op": map[string]interface{}{
					"action": "empty_response_compensation.claim",
					"params": map[string]interface{}{"count": result.ClaimedCount},
				},
				"compensation_ids": result.ClaimedIds,
			},
		})
	}
	return result, nil
}

func GetAdminEmptyResponseCompensations(filter EmptyResponseCompensationAdminFilter) ([]*EmptyResponseCompensation, int64, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if err := expireEmptyResponseCompensations(DB, 0, common.GetTimestamp()); err != nil {
		return nil, 0, err
	}
	query := DB.Model(&EmptyResponseCompensation{}).
		Select("empty_response_compensations.*, users.username").
		Joins("LEFT JOIN users ON users.id = empty_response_compensations.user_id")
	if filter.UserId > 0 {
		query = query.Where("empty_response_compensations.user_id = ?", filter.UserId)
	}
	if filter.ModelName != "" {
		query = query.Where("empty_response_compensations.model_name = ?", filter.ModelName)
	}
	if filter.Status != "" {
		query = query.Where("empty_response_compensations.status = ?", filter.Status)
	}
	if filter.StartTime > 0 {
		query = query.Where("empty_response_compensations.created_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("empty_response_compensations.created_at <= ?", filter.EndTime)
	}
	var total int64
	countQuery := DB.Model(&EmptyResponseCompensation{})
	if filter.UserId > 0 {
		countQuery = countQuery.Where("user_id = ?", filter.UserId)
	}
	if filter.ModelName != "" {
		countQuery = countQuery.Where("model_name = ?", filter.ModelName)
	}
	if filter.Status != "" {
		countQuery = countQuery.Where("status = ?", filter.Status)
	}
	if filter.StartTime > 0 {
		countQuery = countQuery.Where("created_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		countQuery = countQuery.Where("created_at <= ?", filter.EndTime)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []*EmptyResponseCompensation
	if err := query.Order("empty_response_compensations.created_at desc, empty_response_compensations.id desc").
		Offset(filter.Offset).Limit(filter.Limit).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func GetEmptyResponseCompensationAdminSummary() (*EmptyResponseCompensationAdminSummary, error) {
	if err := expireEmptyResponseCompensations(DB, 0, common.GetTimestamp()); err != nil {
		return nil, err
	}
	var rows []struct {
		Status string
		Count  int64
		Quota  int64
	}
	if err := DB.Model(&EmptyResponseCompensation{}).
		Select("status, COUNT(*) AS count, COALESCE(SUM(compensation_quota), 0) AS quota").
		Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	summary := &EmptyResponseCompensationAdminSummary{}
	for _, row := range rows {
		summary.TotalCount += row.Count
		switch row.Status {
		case EmptyResponseCompensationStatusPending:
			summary.PendingCount = row.Count
			summary.PendingQuota = row.Quota
		case EmptyResponseCompensationStatusClaimed:
			summary.ClaimedCount = row.Count
			summary.ClaimedQuota = row.Quota
		case EmptyResponseCompensationStatusBlocked:
			summary.BlockedCount = row.Count
		case EmptyResponseCompensationStatusExpired:
			summary.ExpiredCount = row.Count
		}
	}
	return summary, nil
}
