package suno

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
}

// ParseTaskResult is not used for Suno tasks.
// Suno polling uses a dedicated batch-fetch path (service.UpdateSunoTasks) that
// receives dto.TaskResponse[[]dto.SunoDataResponse] from the upstream /fetch API.
// This differs from the per-task polling used by video adaptors.
func (a *TaskAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, fmt.Errorf("suno uses batch polling via UpdateSunoTasks, ParseTaskResult is not applicable")
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	action := strings.ToUpper(c.Param("action"))

	var sunoRequest *dto.SunoSubmitReq
	err := common.UnmarshalBodyReusable(c, &sunoRequest)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}
	err = actionValidate(c, sunoRequest, action, info.ChannelBaseUrl)
	if err != nil {
		statusCode := http.StatusBadRequest
		errorCode := "invalid_request"
		if isSunoMerchantAPI(info.ChannelBaseUrl) && action == constant.SunoActionLyrics {
			statusCode = http.StatusNotImplemented
			errorCode = "not_implemented"
		}
		taskErr = service.TaskErrorWrapperLocal(err, errorCode, statusCode)
		return
	}

	//if sunoRequest.ContinueClipId != "" {
	//	if sunoRequest.TaskID == "" {
	//		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task id is empty"), "invalid_request", http.StatusBadRequest)
	//		return
	//	}
	//	info.OriginTaskID = sunoRequest.TaskID
	//}

	info.Action = action
	c.Set("task_request", sunoRequest)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	if isSunoMerchantAPI(baseURL) {
		if info.Action == constant.SunoActionLyrics {
			return "", fmt.Errorf("merchant Suno API does not support lyrics submission")
		}
		return strings.TrimRight(sunoMerchantAPIBaseURL(baseURL), "/") + "/music/generate", nil
	}
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, "/suno/submit/"+info.Action)
	return fullRequestURL, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	contentType := c.Request.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	if accept := c.Request.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	sunoRequest, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("task_request not found in context")
	}
	data, err := common.Marshal(sunoRequest)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	if isSunoMerchantAPI(info.ChannelBaseUrl) {
		merchantResponse := merchantGenerateResponse{}
		if err = common.Unmarshal(responseBody, &merchantResponse); err != nil {
			taskErr = service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
			return
		}
		merchantTaskIDs := normalizeMerchantTaskIDs(merchantResponse.Data.TaskIDs)
		if len(merchantTaskIDs) == 0 {
			message := merchantResponse.Message
			if message == "" {
				message = "merchant Suno response did not contain task_ids"
			}
			taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", message), "fail_to_submit_task", http.StatusBadGateway)
			return
		}

		taskID = strings.Join(merchantTaskIDs, ",")
		publicResponse := dto.TaskResponse[string]{
			Code:    dto.TaskSuccessCode,
			Message: merchantResponse.Message,
			Data:    info.PublicTaskID,
		}
		c.JSON(http.StatusOK, publicResponse)
		return taskID, nil, nil
	}

	var sunoResponse dto.TaskResponse[string]
	err = common.Unmarshal(responseBody, &sunoResponse)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if !sunoResponse.IsSuccess() {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", sunoResponse.Message), sunoResponse.Code, http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 替换上游 ID 返回给客户端
	publicResponse := dto.TaskResponse[string]{
		Code:    sunoResponse.Code,
		Message: sunoResponse.Message,
		Data:    info.PublicTaskID,
	}
	c.JSON(http.StatusOK, publicResponse)

	return sunoResponse.Data, nil, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	if isSunoMerchantAPI(baseUrl) {
		return fetchSunoMerchantTasks(baseUrl, key, body, proxy)
	}

	requestUrl := fmt.Sprintf("%s/suno/fetch", baseUrl)
	byteBody, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(byteBody))
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task error: %v", err))
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func actionValidate(c *gin.Context, sunoRequest *dto.SunoSubmitReq, action string, baseURL string) (err error) {
	switch action {
	case constant.SunoActionMusic:
		if sunoRequest.Mv == "" {
			sunoRequest.Mv = "chirp-v3-0"
			if isSunoMerchantAPI(baseURL) {
				sunoRequest.Mv = "chirp-hawk"
			}
		}
	case constant.SunoActionLyrics:
		if isSunoMerchantAPI(baseURL) {
			return fmt.Errorf("merchant Suno API does not support lyrics submission")
		}
		if sunoRequest.Prompt == "" {
			err = fmt.Errorf("prompt_empty")
			return
		}
	default:
		err = fmt.Errorf("invalid_action")
	}
	return
}

// merchantGenerateResponse is the submit response returned by open.suno.cn.
// The provider returns two numeric task IDs for one generation request. NewAPI
// keeps one public task per request and stores the upstream IDs as a comma-
// separated value so polling can return both generated clips together.
type merchantGenerateResponse struct {
	Message string `json:"message"`
	Data    struct {
		TaskIDs []any `json:"task_ids"`
	} `json:"data"`
}

type merchantTaskResponse struct {
	Message string           `json:"message"`
	Data    merchantTaskData `json:"data"`
}

type merchantTaskData struct {
	TaskID       any                 `json:"task_id"`
	Status       string              `json:"status"`
	Result       *merchantTaskResult `json:"result"`
	ErrorMsg     string              `json:"errormsg"`
	ErrorMessage string              `json:"error_message"`
}

type merchantTaskResult struct {
	CustomID          string                `json:"custom_id"`
	Title             string                `json:"title"`
	Text              string                `json:"text"`
	ModelName         string                `json:"model_name"`
	MajorModelVersion string                `json:"major_model_version"`
	FileInfo          *merchantTaskFileInfo `json:"fileInfo"`
}

type merchantTaskFileInfo struct {
	MP3URL   string  `json:"mp3Url"`
	COSURL   string  `json:"cosUrl"`
	Duration float64 `json:"duration"`
}

func isSunoMerchantAPI(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	path := strings.TrimRight(strings.ToLower(parsed.Path), "/")
	if strings.HasSuffix(path, "/api/v1") {
		return true
	}
	return strings.EqualFold(parsed.Hostname(), "open.suno.cn")
}

func sunoMerchantAPIBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err == nil && strings.EqualFold(parsed.Hostname(), "open.suno.cn") {
		parsed.Path = "/api/v1"
		parsed.RawPath = ""
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return strings.TrimRight(parsed.String(), "/")
	}
	if strings.HasSuffix(strings.ToLower(trimmed), "/api/v1") {
		return trimmed
	}
	return trimmed + "/api/v1"
}

func normalizeMerchantTaskIDs(values []any) []string {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		var id string
		switch typed := value.(type) {
		case string:
			id = strings.TrimSpace(typed)
		case float64:
			if typed == float64(int64(typed)) {
				id = strconv.FormatInt(int64(typed), 10)
			}
		case int:
			id = strconv.Itoa(typed)
		case int64:
			id = strconv.FormatInt(typed, 10)
		default:
			continue
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func extractSunoTaskIDs(body map[string]any) []string {
	value, ok := body["ids"]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return normalizeMerchantTaskIDsFromStrings(typed)
	case []any:
		return normalizeMerchantTaskIDs(typed)
	case string:
		parts := strings.Split(typed, ",")
		return normalizeMerchantTaskIDsFromStrings(parts)
	default:
		return nil
	}
}

func normalizeMerchantTaskIDsFromStrings(values []string) []string {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		if id := strings.TrimSpace(value); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func fetchSunoMerchantTasks(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	ids := extractSunoTaskIDs(body)
	if len(ids) == 0 {
		return nil, fmt.Errorf("suno task ids are empty")
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}

	items := make([]dto.SunoDataResponse, 0, len(ids))
	for _, idGroup := range ids {
		atomicIDs := splitMerchantTaskIDs(idGroup)
		if len(atomicIDs) == 0 {
			continue
		}
		groupItems := make([]dto.SunoDataResponse, 0, len(atomicIDs))
		for _, id := range atomicIDs {
			requestURL := strings.TrimRight(sunoMerchantAPIBaseURL(baseURL), "/") + "/music/task?id=" + url.QueryEscape(id)
			req, err := http.NewRequest(http.MethodGet, requestURL, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Authorization", "Bearer "+key)
			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			responseBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("merchant Suno task request returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
			}
			var merchantResponse merchantTaskResponse
			if err := common.Unmarshal(responseBody, &merchantResponse); err != nil {
				return nil, fmt.Errorf("unmarshal merchant Suno task response: %w", err)
			}
			groupItems = append(groupItems, merchantTaskToSunoData(id, merchantResponse))
		}
		if len(groupItems) == 1 {
			items = append(items, groupItems[0])
		} else {
			items = append(items, mergeMerchantTaskData(idGroup, groupItems))
		}
	}

	payload, err := common.Marshal(dto.TaskResponse[[]dto.SunoDataResponse]{
		Code: dto.TaskSuccessCode,
		Data: items,
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}, nil
}

func splitMerchantTaskIDs(value string) []string {
	parts := strings.Split(value, ",")
	return normalizeMerchantTaskIDsFromStrings(parts)
}

func mergeMerchantTaskData(taskID string, items []dto.SunoDataResponse) dto.SunoDataResponse {
	merged := dto.SunoDataResponse{
		TaskID: taskID,
		Action: constant.SunoActionMusic,
		Status: "QUEUED",
	}
	var songs []dto.SunoSong
	for _, item := range items {
		switch item.Status {
		case "IN_PROGRESS":
			merged.Status = "IN_PROGRESS"
		case "SUCCESS":
			if merged.Status != "IN_PROGRESS" {
				merged.Status = "SUCCESS"
			}
		case "FAILURE":
			if merged.Status != "IN_PROGRESS" && merged.Status != "SUCCESS" {
				merged.Status = "FAILURE"
			}
		}
		if merged.FailReason == "" && item.FailReason != "" {
			merged.FailReason = item.FailReason
		}
		var itemSongs []dto.SunoSong
		if err := common.Unmarshal(item.Data, &itemSongs); err == nil {
			songs = append(songs, itemSongs...)
		}
	}
	merged.Data, _ = common.Marshal(songs)
	return merged
}

func merchantTaskToSunoData(taskID string, response merchantTaskResponse) dto.SunoDataResponse {
	status := merchantStatusToSunoStatus(response.Data.Status)
	failReason := strings.TrimSpace(response.Data.ErrorMsg)
	if failReason == "" {
		failReason = strings.TrimSpace(response.Data.ErrorMessage)
	}
	if failReason == "" && status == "FAILURE" {
		failReason = strings.TrimSpace(response.Message)
	}

	songs := make([]dto.SunoSong, 0, 1)
	if response.Data.Result != nil {
		result := response.Data.Result
		song := dto.SunoSong{
			ID:                result.CustomID,
			Title:             result.Title,
			Text:              result.Text,
			ModelName:         result.ModelName,
			MajorModelVersion: result.MajorModelVersion,
			Status:            strings.ToLower(response.Data.Status),
		}
		if result.FileInfo != nil {
			song.AudioURL = result.FileInfo.MP3URL
			song.ImageURL = result.FileInfo.COSURL
			song.Metadata.Duration = result.FileInfo.Duration
		}
		songs = append(songs, song)
	}
	data, _ := common.Marshal(songs)
	return dto.SunoDataResponse{
		TaskID:     taskID,
		Action:     constant.SunoActionMusic,
		Status:     status,
		FailReason: failReason,
		Data:       data,
	}
}

func merchantStatusToSunoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued":
		return "QUEUED"
	case "processing", "running":
		return "IN_PROGRESS"
	case "completed", "success", "succeeded":
		return "SUCCESS"
	case "failed", "failure", "error":
		return "FAILURE"
	default:
		return "IN_PROGRESS"
	}
}
