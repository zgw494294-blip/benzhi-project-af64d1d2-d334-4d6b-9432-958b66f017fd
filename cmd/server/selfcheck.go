package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type smokeClient struct {
	base   string
	client *http.Client
}

func (c smokeClient) post(path string, body map[string]any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s 返回 %d: %s", path, res.StatusCode, string(raw))
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (c smokeClient) get(path string) (map[string]any, error) {
	res, err := c.client.Get(c.base + path)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s 返回 %d: %s", path, res.StatusCode, string(raw))
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
func m(version int, actor, role, key string) map[string]any {
	return map[string]any{"expectedVersion": version, "idempotencyKey": key, "actor": actor, "role": role}
}
func merge(a map[string]any, b map[string]any) map[string]any {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func runSelfcheck(ctx context.Context, addr string) error {
	c := smokeClient{base: "http://" + addr, client: &http.Client{Timeout: 3 * time.Second}}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := c.get("/api/v1/health"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return errorsNew("服务健康检查超时")
		}
		time.Sleep(25 * time.Millisecond)
	}
	uiResponse, err := c.client.Get(c.base + "/")
	if err != nil {
		return fmt.Errorf("浏览器工作台不可达: %w", err)
	}
	uiBody, readErr := io.ReadAll(uiResponse.Body)
	_ = uiResponse.Body.Close()
	if readErr != nil || uiResponse.StatusCode != http.StatusOK || !bytes.Contains(uiBody, []byte("TapeMaster Gate")) {
		return errorsNew("浏览器工作台首页自检失败")
	}
	created, err := c.post("/api/v1/jobs", merge(m(0, "自检采集员", "operator", "self-create"), map[string]any{"title": "自检磁带作业", "collectionRef": "SELF-CHECK", "profile": map[string]any{"sampleRate": 96000, "bitDepth": 24, "channels": 2, "namingRule": "{carrierCode}_master.wav"}}))
	if err != nil {
		return err
	}
	job := created["jobId"].(string)
	version := int(created["version"].(float64))
	carrier, err := c.post("/api/v1/jobs/"+job+"/carriers", merge(m(version, "自检采集员", "operator", "self-carrier"), map[string]any{"carrierCode": "SELF-TAPE-01", "format": "Compact Cassette", "expectedDurationMs": 60000, "conditionGrade": "good", "cleaningRequired": false, "assessmentNote": "自检载体"}))
	if err != nil {
		return err
	}
	version = int(carrier["version"].(float64))
	carrierID := carrier["resourceId"].(string)
	ready, err := c.post("/api/v1/jobs/"+job+"/preflight", merge(m(version, "自检采集员", "operator", "self-preflight"), map[string]any{"playbackCalibrated": true, "storageAvailable": true, "carrierChecks": []map[string]any{{"carrierId": carrierID, "cleaningCompleted": true, "appearancePassed": true, "playbackCompatible": true, "dispositionNote": "自检处置完成", "dispositionCompleted": true}}}))
	if err != nil {
		return err
	}
	version = int(ready["version"].(float64))
	captured, err := c.post("/api/v1/jobs/"+job+"/captures", merge(m(version, "自检采集员", "operator", "self-capture"), map[string]any{"carrierId": carrierID, "sampleRate": 96000, "bitDepth": 24, "channels": 2, "durationMs": 60000, "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "operator": "自检采集员", "contentSummary": "自检音频", "metrics": map[string]any{"peakDbfs": -3, "dropoutCount": 0, "longestSilenceMs": 0}}))
	if err != nil {
		return err
	}
	version = int(captured["version"].(float64))
	submitted, err := c.post("/api/v1/jobs/"+job+"/submit", m(version, "自检审校员", "reviewer", "self-submit"))
	if err != nil {
		return err
	}
	version = int(submitted["version"].(float64))
	preview, err := c.get("/api/v1/jobs/" + job + "/manifest/preview")
	if err != nil {
		return err
	}
	freezeBody := m(version, "自检负责人", "manager", "self-freeze")
	freezeBody["previewVersion"] = int(preview["jobVersion"].(float64))
	freezeBody["previewDigest"] = preview["proposedDigest"].(string)
	frozen, err := c.post("/api/v1/jobs/"+job+"/manifest/freeze", freezeBody)
	if err != nil {
		return err
	}
	version = int(frozen["version"].(float64))
	if _, err = c.post("/api/v1/jobs/"+job+"/certificate", m(version, "自检负责人", "manager", "self-certificate")); err != nil {
		return err
	}
	detail, err := c.get("/api/v1/jobs/" + job)
	if err != nil {
		return err
	}
	cert := detail["certificate"].(map[string]any)
	number := cert["certificateNo"].(string)
	code := cert["verificationCode"].(string)
	verified, err := c.get("/api/v1/certificates/" + number + "/verify?code=" + code)
	if err != nil {
		return err
	}
	if ok, _ := verified["valid"].(bool); !ok {
		return errorsNew("凭据验证未通过")
	}
	audit, err := c.get("/api/v1/jobs/" + job + "/audit")
	if err != nil {
		return err
	}
	if len(audit["timeline"].([]any)) < 7 {
		return errorsNew("审计时间线不完整")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

type simpleError string

func (e simpleError) Error() string { return string(e) }
func errorsNew(v string) error      { return simpleError(v) }
