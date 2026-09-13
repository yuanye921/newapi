package suno

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

func TestSunoMerchantAPIBaseURLDetection(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "official host", url: "https://open.suno.cn", want: true},
		{name: "explicit api path", url: "https://merchant.example/api/v1/", want: true},
		{name: "legacy proxy", url: "https://proxy.example", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isSunoMerchantAPI(tt.url))
		})
	}
}

func TestNormalizeMerchantTaskIDsDropsInvalidValues(t *testing.T) {
	require.Equal(t, []string{"204", "205"}, normalizeMerchantTaskIDs([]any{float64(204), "205", nil, true}))
}

func TestSunoMerchantRequestURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	merchantInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://open.suno.cn"}}
	merchantInfo.TaskRelayInfo = &relaycommon.TaskRelayInfo{Action: "MUSIC"}

	url, err := adaptor.BuildRequestURL(merchantInfo)
	require.NoError(t, err)
	require.Equal(t, "https://open.suno.cn/api/v1/music/generate", url)

	dashboardInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://open.suno.cn/merchant/dashboard"}}
	dashboardInfo.TaskRelayInfo = &relaycommon.TaskRelayInfo{Action: "MUSIC"}
	url, err = adaptor.BuildRequestURL(dashboardInfo)
	require.NoError(t, err)
	require.Equal(t, "https://open.suno.cn/api/v1/music/generate", url)

	legacyInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://proxy.example"}}
	legacyInfo.TaskRelayInfo = &relaycommon.TaskRelayInfo{Action: "MUSIC"}
	url, err = adaptor.BuildRequestURL(legacyInfo)
	require.NoError(t, err)
	require.Equal(t, "https://proxy.example/suno/submit/MUSIC", url)
}

func TestSunoMerchantDoResponseTracksAllTaskIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://open.suno.cn"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"data":{"task_ids":[204,205]}}`)),
	}

	taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	require.Equal(t, "204,205", taskID)
	require.JSONEq(t, `{"code":"success","message":"","data":"task_public"}`, writer.Body.String())
}

func TestSunoMerchantFetchTaskNormalizesCompletedResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	requestedIDs := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/music/task", r.URL.Path)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		requestedIDs = append(requestedIDs, r.URL.Query().Get("id"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("id") == "204" {
			_, _ = w.Write([]byte(`{"data":{"task_id":204,"status":"completed","result":{"custom_id":"clip-1","title":"Test song","fileInfo":{"mp3Url":"https://cdn.example/song.mp3","cosUrl":"https://cdn.example/cover.png"}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"task_id":205,"status":"completed","result":{"custom_id":"clip-2","title":"Test song 2","fileInfo":{"mp3Url":"https://cdn.example/song-2.mp3"}}}}`))
	}))
	defer server.Close()

	resp, err := (&TaskAdaptor{}).FetchTask(server.URL+"/api/v1", "secret", map[string]any{"ids": []string{"204,205"}}, "")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result dto.TaskResponse[[]dto.SunoDataResponse]
	require.NoError(t, common.Unmarshal(body, &result))
	require.True(t, result.IsSuccess())
	require.Len(t, result.Data, 1)
	require.ElementsMatch(t, []string{"204", "205"}, requestedIDs)
	require.Equal(t, "204,205", result.Data[0].TaskID)
	require.Equal(t, "SUCCESS", result.Data[0].Status)

	var songs []dto.SunoSong
	require.NoError(t, common.Unmarshal(result.Data[0].Data, &songs))
	require.Len(t, songs, 2)
	require.Equal(t, "clip-1", songs[0].ID)
	require.Equal(t, "https://cdn.example/song.mp3", songs[0].AudioURL)
	require.Equal(t, "clip-2", songs[1].ID)
}

func TestSunoMerchantLyricsIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "action", Value: "lyrics"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/suno/submit/lyrics", bytes.NewReader([]byte(`{"prompt":"dance"}`)))

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://open.suno.cn"}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusNotImplemented, taskErr.StatusCode)
}
