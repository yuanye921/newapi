package controller

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"html/template"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const defaultAlipayF2FGateway = "https://openapi.alipay.com/gateway.do"

var alipayF2FCheckoutTemplate = template.Must(template.New("alipay_f2f_checkout").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>支付宝当面付</title>
  <style>
    :root { color-scheme: dark; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: #e8f2ff;
      background:
        radial-gradient(circle at 15% 15%, rgba(22,119,255,.35), transparent 32rem),
        radial-gradient(circle at 90% 10%, rgba(35,211,255,.22), transparent 28rem),
        linear-gradient(135deg, #061224 0%, #0b1c35 45%, #0b2b3e 100%);
    }
    .panel {
      width: min(92vw, 420px);
      padding: 28px;
      border: 1px solid rgba(160, 197, 255, .24);
      border-radius: 18px;
      background: rgba(8, 24, 48, .72);
      box-shadow: 0 24px 80px rgba(0, 0, 0, .32);
      backdrop-filter: blur(18px);
      text-align: center;
    }
    h1 { margin: 0 0 8px; font-size: 22px; letter-spacing: 0; }
    .meta { margin: 0 0 22px; color: #b7cae5; font-size: 14px; line-height: 1.6; }
    .qr {
      width: 260px;
      height: 260px;
      padding: 14px;
      border-radius: 16px;
      background: #fff;
      margin: 0 auto 20px;
    }
    .qr img { width: 100%; height: 100%; display: block; }
    .status {
      min-height: 24px;
      color: #8fc7ff;
      font-size: 14px;
    }
    .trade {
      margin-top: 14px;
      color: #7f98ba;
      font-size: 12px;
      word-break: break-all;
    }
    a { color: #9ed2ff; }
  </style>
</head>
<body>
  <main class="panel">
    <h1>支付宝扫码支付</h1>
    <p class="meta">订单金额：￥{{.Money}}<br>请使用支付宝扫描下方二维码完成付款。</p>
    <div class="qr"><img src="{{.QRCodeDataURL}}" alt="支付宝支付二维码"></div>
    <div id="status" class="status">等待支付中...</div>
    <div class="trade">订单号：{{.TradeNo}}</div>
  </main>
  <script>
    const statusUrl = {{.StatusURLJS}};
    const returnUrl = {{.ReturnURLJS}};
    const statusEl = document.getElementById('status');
    async function poll() {
      try {
        const response = await fetch(statusUrl, { credentials: 'include', cache: 'no-store' });
        const payload = await response.json();
        const status = payload && payload.data && payload.data.status;
        if (status === 'success') {
          statusEl.textContent = '支付成功，正在返回...';
          window.location.href = returnUrl;
          return;
        }
        if (status === 'failed' || status === 'expired') {
          statusEl.textContent = '订单已失效，请重新发起充值。';
          return;
        }
      } catch (error) {
        statusEl.textContent = '正在等待支付结果...';
      }
      window.setTimeout(poll, 3000);
    }
    window.setTimeout(poll, 2500);
  </script>
</body>
</html>`))

type alipayF2FPreCreateResponse struct {
	AlipayTradePrecreateResponse struct {
		Code       string `json:"code"`
		Msg        string `json:"msg"`
		SubCode    string `json:"sub_code"`
		SubMsg     string `json:"sub_msg"`
		OutTradeNo string `json:"out_trade_no"`
		QRCode     string `json:"qr_code"`
	} `json:"alipay_trade_precreate_response"`
	Sign string `json:"sign"`
}

type alipayF2FCheckoutPageData struct {
	TradeNo       string
	Money         string
	QRCodeDataURL template.URL
	StatusURLJS   template.JS
	ReturnURLJS   template.JS
}

func RequestAlipayF2F(c *gin.Context, req EpayRequest) {
	if !isAlipayF2FTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付宝当面付未配置完整"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	callBackAddress := strings.TrimRight(service.GetCallbackAddress(), "/")
	if callBackAddress == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "请先配置服务器地址或自定义回调地址"})
		return
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())
	notifyURL := callBackAddress + "/api/user/alipay-f2f/notify"
	subject := fmt.Sprintf("TUC%d", req.Amount)
	qrCode, rawResponse, err := alipayF2FPreCreate(tradeNo, payMoney, subject, notifyURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝当面付 预下单失败 user_id=%d trade_no=%s amount=%d money=%.2f error=%q raw=%q", id, tradeNo, req.Amount, payMoney, err.Error(), rawResponse))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付宝当面付预下单失败：" + err.Error()})
		return
	}

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAlipayF2F,
		PaymentProvider: model.PaymentProviderAlipayF2F,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err = topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝当面付 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	checkoutURL := callBackAddress + "/api/user/alipay-f2f/checkout"
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝当面付 充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%.2f", id, tradeNo, req.Amount, payMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"url":     checkoutURL,
		"data": gin.H{
			"trade_no": tradeNo,
			"qr_code":  qrCode,
			"money":    strconvFormatMoney(payMoney),
		},
	})
}

func AlipayF2FCheckout(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusBadRequest, "invalid request")
		return
	}
	tradeNo := c.Request.Form.Get("trade_no")
	qrCode := c.Request.Form.Get("qr_code")
	money := c.Request.Form.Get("money")
	returnURL := paymentReturnPath("/console/log?show_history=true")
	if tradeNo == "" || qrCode == "" {
		c.String(http.StatusBadRequest, "missing payment qr code")
		return
	}

	qrDataURL, err := alipayF2FQRCodeDataURL(qrCode)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to render qr code")
		return
	}

	statusURL := "/api/user/alipay-f2f/status/" + url.PathEscape(tradeNo)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = alipayF2FCheckoutTemplate.Execute(c.Writer, alipayF2FCheckoutPageData{
		TradeNo:       tradeNo,
		Money:         money,
		QRCodeDataURL: template.URL(qrDataURL),
		StatusURLJS:   template.JS(jsonString(statusURL)),
		ReturnURLJS:   template.JS(jsonString(returnURL)),
	})
}

func AlipayF2FStatus(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	if tradeNo == "" {
		common.ApiErrorMsg(c, "订单号不能为空")
		return
	}
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderAlipayF2F {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}
	common.ApiSuccess(c, gin.H{"status": topUp.Status})
}

func AlipayF2FNotify(c *gin.Context) {
	if !isAlipayF2FWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝当面付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝当面付 webhook 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	params := make(map[string]string, len(c.Request.Form))
	for key := range c.Request.Form {
		params[key] = c.Request.Form.Get(key)
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝当面付 webhook 收到请求 path=%q client_ip=%s params=%q", c.Request.RequestURI, c.ClientIP(), common.GetJsonString(params)))

	if err := verifyAlipayF2FParams(params); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝当面付 webhook 验签失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if params["app_id"] != "" && params["app_id"] != strings.TrimSpace(operation_setting.AlipayF2FAppId) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝当面付 webhook app_id 不匹配 app_id=%s client_ip=%s", params["app_id"], c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	tradeStatus := params["trade_status"]
	if tradeStatus != "TRADE_SUCCESS" && tradeStatus != "TRADE_FINISHED" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝当面付 webhook 忽略事件 trade_no=%s trade_status=%s client_ip=%s", params["out_trade_no"], tradeStatus, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	tradeNo := params["out_trade_no"]
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝当面付 回调订单不存在 trade_no=%s client_ip=%s", tradeNo, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderAlipayF2F {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝当面付 订单支付网关不匹配 trade_no=%s order_provider=%s client_ip=%s", tradeNo, topUp.PaymentProvider, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if topUp.Status == common.TopUpStatusSuccess {
		_, _ = c.Writer.Write([]byte("success"))
		return
	}
	if topUp.Status != common.TopUpStatusPending {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝当面付 订单状态不是待支付 trade_no=%s status=%s client_ip=%s", tradeNo, topUp.Status, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if !alipayF2FMoneyMatches(topUp.Money, params["total_amount"]) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝当面付 回调金额不匹配 trade_no=%s expected=%.2f actual=%s client_ip=%s", tradeNo, topUp.Money, params["total_amount"], c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	topUp.Status = common.TopUpStatusSuccess
	topUp.CompleteTime = common.GetTimestamp()
	if err := topUp.Update(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝当面付 更新充值订单失败 trade_no=%s user_id=%d client_ip=%s error=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	quotaToAdd := int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
	if quotaToAdd <= 0 {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝当面付 无效充值额度 trade_no=%s user_id=%d amount=%d", topUp.TradeNo, topUp.UserId, topUp.Amount))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if err := model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝当面付 更新用户额度失败 trade_no=%s user_id=%d client_ip=%s quota_to_add=%d error=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), quotaToAdd, err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	model.RecordTopupLog(topUp.UserId, fmt.Sprintf("使用支付宝当面付充值成功，充值金额: %v，支付金额：%.2f", logger.LogQuota(quotaToAdd), topUp.Money), c.ClientIP(), topUp.PaymentMethod, model.PaymentProviderAlipayF2F)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝当面付 充值成功 trade_no=%s user_id=%d client_ip=%s quota_to_add=%d money=%.2f", topUp.TradeNo, topUp.UserId, c.ClientIP(), quotaToAdd, topUp.Money))
	_, _ = c.Writer.Write([]byte("success"))
}

func alipayF2FPreCreate(tradeNo string, money float64, subject string, notifyURL string) (string, string, error) {
	bizContent, err := json.Marshal(map[string]string{
		"out_trade_no": tradeNo,
		"total_amount": strconvFormatMoney(money),
		"subject":      subject,
	})
	if err != nil {
		return "", "", err
	}

	params := map[string]string{
		"app_id":      strings.TrimSpace(operation_setting.AlipayF2FAppId),
		"method":      "alipay.trade.precreate",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"notify_url":  notifyURL,
		"biz_content": string(bizContent),
	}
	sign, err := signAlipayF2FParams(params)
	if err != nil {
		return "", "", err
	}
	params["sign"] = sign

	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequest(http.MethodPost, alipayF2FGateway(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	raw := string(body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", raw, fmt.Errorf("支付宝接口 HTTP %d", resp.StatusCode)
	}

	var parsed alipayF2FPreCreateResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", raw, err
	}
	payResponse := parsed.AlipayTradePrecreateResponse
	if payResponse.Code != "10000" {
		msg := strings.TrimSpace(payResponse.SubMsg)
		if msg == "" {
			msg = strings.TrimSpace(payResponse.Msg)
		}
		if msg == "" {
			msg = "支付宝接口返回失败"
		}
		return "", raw, errors.New(msg)
	}
	if payResponse.QRCode == "" {
		return "", raw, errors.New("支付宝未返回二维码")
	}
	if payResponse.OutTradeNo != "" && payResponse.OutTradeNo != tradeNo {
		return "", raw, errors.New("支付宝返回的订单号不匹配")
	}
	return payResponse.QRCode, raw, nil
}

func alipayF2FGateway() string {
	gateway := strings.TrimSpace(operation_setting.AlipayF2FGateway)
	if gateway == "" {
		return defaultAlipayF2FGateway
	}
	return gateway
}

func signAlipayF2FParams(params map[string]string) (string, error) {
	privateKey, err := parseAlipayF2FPrivateKey(operation_setting.AlipayF2FAppPrivateKey)
	if err != nil {
		return "", err
	}
	signContent := canonicalAlipayF2FParams(params, true)
	hashType, digest := alipayF2FDigest(signContent, params["sign_type"])
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, hashType, digest)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func verifyAlipayF2FParams(params map[string]string) error {
	signature := strings.TrimSpace(params["sign"])
	if signature == "" {
		return errors.New("missing sign")
	}
	publicKey, err := parseAlipayF2FPublicKey(operation_setting.AlipayF2FAlipayPublicKey)
	if err != nil {
		return err
	}
	signBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return err
	}
	signContent := canonicalAlipayF2FParams(params, false)
	hashType, digest := alipayF2FDigest(signContent, params["sign_type"])
	return rsa.VerifyPKCS1v15(publicKey, hashType, digest, signBytes)
}

func canonicalAlipayF2FParams(params map[string]string, includeSignType bool) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "sign" || value == "" {
			continue
		}
		if !includeSignType && key == "sign_type" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return strings.Join(parts, "&")
}

func alipayF2FDigest(content string, signType string) (crypto.Hash, []byte) {
	if strings.EqualFold(signType, "RSA") {
		sum := sha1.Sum([]byte(content))
		return crypto.SHA1, sum[:]
	}
	sum := sha256.Sum256([]byte(content))
	return crypto.SHA256, sum[:]
}

func parseAlipayF2FPrivateKey(raw string) (*rsa.PrivateKey, error) {
	der, err := alipayF2FKeyDER(raw)
	if err != nil {
		return nil, err
	}
	if privateKey, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return privateKey, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	privateKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return privateKey, nil
}

func parseAlipayF2FPublicKey(raw string) (*rsa.PublicKey, error) {
	der, err := alipayF2FKeyDER(raw)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err == nil {
		publicKey, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("public key is not RSA")
		}
		return publicKey, nil
	}
	if publicKey, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return publicKey, nil
	}
	return nil, err
}

func alipayF2FKeyDER(raw string) ([]byte, error) {
	material := strings.TrimSpace(strings.ReplaceAll(raw, "\\n", "\n"))
	if material == "" {
		return nil, errors.New("empty key")
	}
	if strings.Contains(material, "BEGIN") {
		block, _ := pem.Decode([]byte(material))
		if block == nil {
			return nil, errors.New("invalid pem key")
		}
		return block.Bytes, nil
	}
	material = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, material)
	return base64.StdEncoding.DecodeString(material)
}

func alipayF2FQRCodeDataURL(value string) (string, error) {
	code, err := qr.Encode(value, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	code, err = barcode.Scale(code, 256, 256)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, code); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func alipayF2FMoneyMatches(expected float64, actual string) bool {
	actualDecimal, err := decimal.NewFromString(strings.TrimSpace(actual))
	if err != nil {
		return false
	}
	expectedDecimal := decimal.NewFromFloat(expected).Round(2)
	return actualDecimal.Round(2).Equal(expectedDecimal)
}

func strconvFormatMoney(value float64) string {
	return decimal.NewFromFloat(value).Round(2).StringFixed(2)
}

func jsonString(value string) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(bytes)
}
