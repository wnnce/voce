package translator

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const (
	ProviderVolcengine   = "volcengine"
	defaultVolcEndpoint  = "https://translate.volcengineapi.com"
	defaultVolcRegion    = "cn-north-1"
	volcServiceTranslate = "translate"
	volcActionTranslate  = "TranslateText"
	volcVersionTranslate = "2020-06-01"
)

type VolcengineProvider struct {
	accessKey string
	secretKey string
	region    string
	endpoint  string
	client    *http.Client
}

type volcTranslateRequest struct {
	TargetLanguage string   `json:"TargetLanguage"`
	TextList       []string `json:"TextList"`
	SourceLanguage string   `json:"SourceLanguage,omitempty"`
}

type volcTranslateResponse struct {
	ResponseMetadata struct {
		RequestId string `json:"RequestId"`
		Action    string `json:"Action"`
		Version   string `json:"Version"`
		Service   string `json:"Service"`
		Region    string `json:"Region"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"ResponseMetadata"`
	TranslationList []struct {
		Translation            string `json:"Translation"`
		DetectedSourceLanguage string `json:"DetectedSourceLanguage"`
	} `json:"TranslationList"`
}

func NewVolcengineProvider(_ context.Context, properties json.RawMessage) (Provider, error) {
	var raw map[string]any
	if len(properties) > 0 {
		if err := sonic.Unmarshal(properties, &raw); err != nil {
			return nil, fmt.Errorf("volcengine: invalid properties JSON: %w", err)
		}
	} else {
		raw = make(map[string]any)
	}

	accessKey, _ := raw["access_key"].(string)
	if accessKey == "" {
		return nil, errors.New("volcengine: access_key is required in properties")
	}

	secretKey, _ := raw["secret_key"].(string)
	if secretKey == "" {
		return nil, errors.New("volcengine: secret_key is required in properties")
	}

	region, _ := raw["region"].(string)
	if region == "" {
		region = defaultVolcRegion
	}

	endpoint, _ := raw["endpoint"].(string)
	if endpoint == "" {
		endpoint = defaultVolcEndpoint
	}

	timeoutMs := 5000
	if t, ok := raw["timeout_ms"].(float64); ok && t > 0 {
		timeoutMs = int(t)
	}

	return &VolcengineProvider{
		accessKey: accessKey,
		secretKey: secretKey,
		region:    region,
		endpoint:  endpoint,
		client: &http.Client{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		},
	}, nil
}

func (v *VolcengineProvider) Name() string {
	return ProviderVolcengine
}

func (v *VolcengineProvider) Close() error {
	return nil
}

func (v *VolcengineProvider) TranslateStream(
	ctx context.Context,
	text string,
	opts TranslateOptions,
	handler StreamHandler,
) error {
	if strings.TrimSpace(text) == "" {
		return handler("", "", true)
	}

	targetLang := ToVolcengineLang(opts.TargetLang)
	if targetLang == "" {
		targetLang = "en"
	}
	sourceLang := ToVolcengineLang(opts.SourceLang)

	reqPayload := volcTranslateRequest{
		TargetLanguage: targetLang,
		TextList:       []string{text},
		SourceLanguage: sourceLang,
	}

	bodyBytes, err := sonic.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("volcengine: marshal request error: %w", err)
	}

	endpointURL, err := url.Parse(v.endpoint)
	if err != nil {
		return fmt.Errorf("volcengine: invalid endpoint URL: %w", err)
	}
	if endpointURL.Path == "" {
		endpointURL.Path = "/"
	}

	q := endpointURL.Query()
	q.Set("Action", volcActionTranslate)
	q.Set("Version", volcVersionTranslate)
	endpointURL.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("volcengine: new request error: %w", err)
	}

	httpReq.Host = endpointURL.Host
	signVolcengineRequest(httpReq, bodyBytes, v.accessKey, v.secretKey, v.region, volcServiceTranslate, time.Now())

	start := time.Now()
	resp, err := v.client.Do(httpReq)
	if err != nil {
		slog.ErrorContext(ctx, "volcengine translation request failed",
			"network_latency_ms", time.Since(start).Milliseconds(),
			"error", err,
			"text", text,
		)
		return fmt.Errorf("volcengine: http post error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	networkLatency := time.Since(start)
	if err != nil {
		return fmt.Errorf("volcengine: read response body error: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.ErrorContext(ctx, "volcengine translation api error",
			"status", resp.StatusCode,
			"network_latency_ms", networkLatency.Milliseconds(),
			"body", string(respBytes),
		)
		return fmt.Errorf("volcengine: api error status %d: %s", resp.StatusCode, string(respBytes))
	}

	var parsed volcTranslateResponse
	if err := sonic.Unmarshal(respBytes, &parsed); err != nil {
		return fmt.Errorf("volcengine: unmarshal response error: %w", err)
	}

	if parsed.ResponseMetadata.Error != nil {
		slog.ErrorContext(ctx, "volcengine translation metadata error",
			"code", parsed.ResponseMetadata.Error.Code,
			"message", parsed.ResponseMetadata.Error.Message,
			"network_latency_ms", networkLatency.Milliseconds(),
		)
		return fmt.Errorf("volcengine: %s - %s", parsed.ResponseMetadata.Error.Code, parsed.ResponseMetadata.Error.Message)
	}

	if len(parsed.TranslationList) == 0 {
		return errors.New("volcengine: empty translations returned")
	}

	res := parsed.TranslationList[0].Translation
	slog.InfoContext(ctx, "volcengine translation finished",
		"network_latency_ms", networkLatency.Milliseconds(),
		"is_final", opts.IsFinal,
		"text", text,
		"translation", res,
	)
	return handler(res, res, true)
}

func hmacSHA256(key []byte, content string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(content))
	return mac.Sum(nil)
}

func hashSHA256(data []byte) string {
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

func getVolcSigningKey(secretKey, date, region, service string) []byte {
	kDate := hmacSHA256([]byte(secretKey), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "request")
}

func signVolcengineRequest(
	req *http.Request,
	bodyBytes []byte,
	accessKey, secretKey, region, service string,
	t time.Time,
) {
	dateStamp := t.UTC().Format("20060102")
	amzDate := t.UTC().Format("20060102T150405Z")

	payloadHash := hashSHA256(bodyBytes)
	req.Header.Set("X-Date", amzDate)
	req.Header.Set("X-Content-Sha256", payloadHash)
	req.Header.Set("Content-Type", "application/json")

	headers := map[string]string{
		"content-type":     "application/json",
		"host":             req.URL.Host,
		"x-content-sha256": payloadHash,
		"x-date":           amzDate,
	}

	sortedKeys := make([]string, 0, len(headers))
	for k := range headers {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	var canonicalHeadersBuilder strings.Builder
	var signedHeadersBuilder strings.Builder
	for i, k := range sortedKeys {
		canonicalHeadersBuilder.WriteString(fmt.Sprintf("%s:%s\n", k, strings.TrimSpace(headers[k])))
		signedHeadersBuilder.WriteString(k)
		if i < len(sortedKeys)-1 {
			signedHeadersBuilder.WriteString(";")
		}
	}
	canonicalHeaders := canonicalHeadersBuilder.String()
	signedHeaders := signedHeadersBuilder.String()

	canonicalQueryString := req.URL.Query().Encode()
	path := req.URL.Path
	if path == "" {
		path = "/"
	}

	canonicalRequest := strings.Join([]string{
		req.Method,
		path,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	algorithm := "HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/%s/request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		algorithm,
		amzDate,
		credentialScope,
		hashSHA256([]byte(canonicalRequest)),
	}, "\n")

	signingKey := getVolcSigningKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, accessKey, credentialScope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authorization)
}

func init() {
	RegisterProvider(ProviderVolcengine, NewVolcengineProvider)
}
