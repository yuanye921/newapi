package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	emptysetting "github.com/QuantumNous/new-api/setting/empty_response_compensation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetEmptyResponseCompensationTestData(t *testing.T) {
	t.Helper()
	tables := []string{"empty_response_compensations", "redemptions", "top_ups", "logs", "users"}
	for _, table := range tables {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range tables {
			DB.Exec("DELETE FROM " + table)
		}
	})
}

func emptyResponseCompensationTestSetting() emptysetting.Setting {
	return emptysetting.Setting{
		Enabled:                true,
		ModelRatios:            map[string]int{"test-model": 50},
		MinQualificationAmount: 50,
		InputTokenThreshold:    50,
		OutputTokenThreshold:   9,
		ClaimWindowDays:        7,
		DailyClaimLimit:        0,
		OverclockWindowMinutes: 0,
		OverclockEmptyCount:    0,
	}
}

func createEmptyResponseCompensationTestUser(t *testing.T, username string, quota int) *User {
	t.Helper()
	user := &User{
		Username: username,
		Password: "test-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func createEmptyResponseCompensationTestRecord(t *testing.T, userId int, requestId string, originalQuota int, setting emptysetting.Setting) *EmptyResponseCompensation {
	t.Helper()
	record, err := CreateEmptyResponseCompensation(CreateEmptyResponseCompensationParams{
		UserId:            userId,
		RequestId:         requestId,
		ModelName:         "test-model",
		ChannelId:         1,
		TokenId:           1,
		PromptTokens:      100,
		CompletionTokens:  0,
		OriginalQuota:     originalQuota,
		CompensationRatio: 50,
		Setting:           setting,
	})
	require.NoError(t, err)
	require.NotNil(t, record)
	return record
}

func TestCompensationQuotaRoundsUsingExistingQuotaRules(t *testing.T) {
	tests := []struct {
		original int
		ratio    int
		want     int
	}{
		{original: 100, ratio: 50, want: 50},
		{original: 101, ratio: 50, want: 51},
		{original: 1, ratio: 1, want: 0},
		{original: 100, ratio: 100, want: 100},
		{original: 0, ratio: 50, want: 0},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, compensationQuota(test.original, test.ratio))
	}
}

func TestQualificationIncludesSuccessfulTopUpsAndUsedRedemptionsOnly(t *testing.T) {
	resetEmptyResponseCompensationTestData(t)
	user := createEmptyResponseCompensationTestUser(t, "empty-qualification", 100)

	require.NoError(t, DB.Create(&TopUp{
		UserId: user.Id, Amount: 40, TradeNo: "qualified-topup", PaymentProvider: PaymentProviderAlipayF2F, Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: user.Id, Amount: 100, TradeNo: "balance-topup", PaymentProvider: PaymentProviderBalance, Status: common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId: user.Id, Amount: 100, TradeNo: "pending-topup", PaymentProvider: PaymentProviderAlipayF2F, Status: "pending",
	}).Error)
	require.NoError(t, DB.Create(&Redemption{
		Key: "used-redemption-code-0000000001", Status: common.RedemptionCodeStatusUsed, Quota: int(10 * common.QuotaPerUnit), UsedUserId: user.Id,
	}).Error)
	require.NoError(t, DB.Create(&Redemption{
		Key: "unused-redemption-code-00000001", Status: common.RedemptionCodeStatusEnabled, Quota: int(100 * common.QuotaPerUnit), UsedUserId: user.Id,
	}).Error)

	summary, err := GetEmptyResponseCompensationSummary(user.Id, emptyResponseCompensationTestSetting())
	require.NoError(t, err)
	assert.Equal(t, float64(50), summary.QualificationAmount)
	assert.True(t, summary.Qualified)
}

func TestCreateEmptyResponseCompensationIsIdempotentAndBlocksOverclock(t *testing.T) {
	resetEmptyResponseCompensationTestData(t)
	user := createEmptyResponseCompensationTestUser(t, "empty-overclock", 100)
	setting := emptyResponseCompensationTestSetting()
	setting.OverclockWindowMinutes = 10
	setting.OverclockEmptyCount = 3

	for index := 1; index <= 4; index++ {
		record := createEmptyResponseCompensationTestRecord(t, user.Id, "overclock-"+string(rune('0'+index)), 100, setting)
		if index <= 3 {
			assert.Equal(t, EmptyResponseCompensationStatusPending, record.Status)
		} else {
			assert.Equal(t, EmptyResponseCompensationStatusBlocked, record.Status)
			assert.Equal(t, EmptyResponseCompensationBlockHighFrequency, record.BlockReason)
		}
	}

	duplicate, err := CreateEmptyResponseCompensation(CreateEmptyResponseCompensationParams{
		UserId:            user.Id,
		RequestId:         "overclock-1",
		ModelName:         "test-model",
		OriginalQuota:     100,
		CompensationRatio: 50,
		Setting:           setting,
	})
	require.NoError(t, err)
	assert.Nil(t, duplicate)

	var count int64
	require.NoError(t, DB.Model(&EmptyResponseCompensation{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.EqualValues(t, 4, count)
}

func TestConcurrentCreateHonorsOverclockBoundary(t *testing.T) {
	resetEmptyResponseCompensationTestData(t)
	user := createEmptyResponseCompensationTestUser(t, "empty-concurrent-overclock", 100)
	setting := emptyResponseCompensationTestSetting()
	setting.OverclockWindowMinutes = 10
	setting.OverclockEmptyCount = 3
	createEmptyResponseCompensationTestRecord(t, user.Id, "concurrent-overclock-1", 100, setting)
	createEmptyResponseCompensationTestRecord(t, user.Id, "concurrent-overclock-2", 100, setting)

	records := make(chan *EmptyResponseCompensation, 2)
	errors := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for index := 3; index <= 4; index++ {
		waitGroup.Add(1)
		go func(requestIndex int) {
			defer waitGroup.Done()
			record, err := CreateEmptyResponseCompensation(CreateEmptyResponseCompensationParams{
				UserId:            user.Id,
				RequestId:         fmt.Sprintf("concurrent-overclock-%d", requestIndex),
				ModelName:         "test-model",
				OriginalQuota:     100,
				CompensationRatio: 50,
				Setting:           setting,
			})
			records <- record
			errors <- err
		}(index)
	}
	waitGroup.Wait()
	close(records)
	close(errors)

	for err := range errors {
		require.NoError(t, err)
	}
	pending := 0
	blocked := 0
	for record := range records {
		require.NotNil(t, record)
		switch record.Status {
		case EmptyResponseCompensationStatusPending:
			pending++
		case EmptyResponseCompensationStatusBlocked:
			blocked++
		}
	}
	assert.Equal(t, 1, pending)
	assert.Equal(t, 1, blocked)
}

func TestClaimAppliesQualificationDailyLimitAndAuditLog(t *testing.T) {
	resetEmptyResponseCompensationTestData(t)
	user := createEmptyResponseCompensationTestUser(t, "empty-claim", 100)
	setting := emptyResponseCompensationTestSetting()
	setting.DailyClaimLimit = 1
	require.NoError(t, DB.Create(&TopUp{
		UserId: user.Id, Amount: 50, TradeNo: "claim-qualification", PaymentProvider: PaymentProviderAlipayF2F, Status: common.TopUpStatusSuccess,
	}).Error)

	first := createEmptyResponseCompensationTestRecord(t, user.Id, "claim-1", 101, setting)
	second := createEmptyResponseCompensationTestRecord(t, user.Id, "claim-2", 200, setting)

	result, err := ClaimEmptyResponseCompensations(user.Id, []int{first.Id, second.Id}, setting)
	require.NoError(t, err)
	assert.Equal(t, []int{first.Id}, result.ClaimedIds)
	assert.Equal(t, "daily_limit", result.Skipped[second.Id])
	assert.Equal(t, 51, result.CreditedQuota)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 151, reloaded.Quota)

	var auditLog Log
	require.NoError(t, DB.Where("user_id = ? AND type = ?", user.Id, LogTypeRefund).First(&auditLog).Error)
	assert.Equal(t, 51, auditLog.Quota)

	setting.DailyClaimLimit = 0
	result, err = ClaimEmptyResponseCompensations(user.Id, []int{second.Id}, setting)
	require.NoError(t, err)
	assert.Equal(t, []int{second.Id}, result.ClaimedIds)
	assert.Equal(t, 100, result.CreditedQuota)
}

func TestClaimCanBecomeEligibleAfterLaterTopUp(t *testing.T) {
	resetEmptyResponseCompensationTestData(t)
	user := createEmptyResponseCompensationTestUser(t, "empty-later-topup", 100)
	setting := emptyResponseCompensationTestSetting()
	record := createEmptyResponseCompensationTestRecord(t, user.Id, "later-topup", 100, setting)

	result, err := ClaimEmptyResponseCompensations(user.Id, []int{record.Id}, setting)
	require.NoError(t, err)
	assert.Equal(t, "not_qualified", result.Skipped[record.Id])

	require.NoError(t, DB.Create(&TopUp{
		UserId: user.Id, Amount: 50, TradeNo: "later-success", PaymentProvider: PaymentProviderAlipayF2F, Status: common.TopUpStatusSuccess,
	}).Error)
	result, err = ClaimEmptyResponseCompensations(user.Id, []int{record.Id}, setting)
	require.NoError(t, err)
	assert.Equal(t, []int{record.Id}, result.ClaimedIds)
}

func TestClaimUsesLockedAmountAndSkipsExpiredRecords(t *testing.T) {
	resetEmptyResponseCompensationTestData(t)
	user := createEmptyResponseCompensationTestUser(t, "empty-snapshot", 100)
	setting := emptyResponseCompensationTestSetting()
	setting.MinQualificationAmount = 0

	locked := createEmptyResponseCompensationTestRecord(t, user.Id, "locked-ratio", 101, setting)
	setting.ModelRatios["test-model"] = 100
	result, err := ClaimEmptyResponseCompensations(user.Id, []int{locked.Id}, setting)
	require.NoError(t, err)
	assert.Equal(t, 51, result.CreditedQuota)

	expired := createEmptyResponseCompensationTestRecord(t, user.Id, "expired-record", 100, setting)
	require.NoError(t, DB.Model(expired).Update("expires_at", common.GetTimestamp()-1).Error)
	result, err = ClaimEmptyResponseCompensations(user.Id, []int{expired.Id}, setting)
	require.NoError(t, err)
	assert.Equal(t, EmptyResponseCompensationStatusExpired, result.Skipped[expired.Id])
	assert.Zero(t, result.CreditedQuota)
}

func TestConcurrentClaimCreditsRecordOnlyOnce(t *testing.T) {
	resetEmptyResponseCompensationTestData(t)
	user := createEmptyResponseCompensationTestUser(t, "empty-concurrent", 100)
	setting := emptyResponseCompensationTestSetting()
	setting.MinQualificationAmount = 0
	record := createEmptyResponseCompensationTestRecord(t, user.Id, "concurrent-claim", 100, setting)

	results := make(chan *EmptyResponseCompensationClaimResult, 2)
	errors := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			result, err := ClaimEmptyResponseCompensations(user.Id, []int{record.Id}, setting)
			results <- result
			errors <- err
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errors)

	credited := 0
	claimed := 0
	for err := range errors {
		require.NoError(t, err)
	}
	for result := range results {
		require.NotNil(t, result)
		credited += result.CreditedQuota
		claimed += result.ClaimedCount
	}
	assert.Equal(t, 50, credited)
	assert.Equal(t, 1, claimed)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.Equal(t, 150, reloaded.Quota)
}

func TestLocalDayRangeUsesConfiguredServerTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	originalLocation := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = originalLocation })

	start, end := localDayRange(time.Date(2026, 7, 15, 23, 59, 59, 0, location))
	assert.Equal(t, time.Date(2026, 7, 15, 0, 0, 0, 0, location).Unix(), start)
	assert.Equal(t, time.Date(2026, 7, 16, 0, 0, 0, 0, location).Unix(), end)
}
