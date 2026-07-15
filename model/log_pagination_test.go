package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogByTokenIdPaginated(t *testing.T) {
	truncateTables(t)

	for i := 1; i <= 205; i++ {
		require.NoError(t, LOG_DB.Create(&Log{
			TokenId:   42,
			CreatedAt: int64(i),
			ModelName: fmt.Sprintf("model-%03d", i),
		}).Error)
	}
	require.NoError(t, LOG_DB.Create(&Log{TokenId: 99, CreatedAt: 999, ModelName: "other-token"}).Error)

	logs, total, err := GetLogByTokenIdPaginated(42, 100, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 205, total)
	require.Len(t, logs, 100)
	assert.Equal(t, "model-105", logs[0].ModelName)
	assert.Equal(t, "model-006", logs[99].ModelName)
	assert.Equal(t, 101, logs[0].Id)
	assert.Equal(t, 200, logs[99].Id)

	lastPage, total, err := GetLogByTokenIdPaginated(42, 200, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 205, total)
	require.Len(t, lastPage, 5)
	assert.Equal(t, "model-005", lastPage[0].ModelName)
	assert.Equal(t, "model-001", lastPage[4].ModelName)
}
