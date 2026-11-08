package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"xiaotao/xt-client/internal/logging"
)

// SkipReason records why an account was not selected for a model.
type SkipReason struct {
	Account  string
	Provider string
	Model    string
	Reason   string // "disabled", "model_unsupported", "quota_exhausted", "cooling"
}

func randHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func redact(s string) string {
	return logging.Redact(s, 0)
}

func logProxyWarn(msg string, fields ...logging.Field) {
	logging.Default().For(logging.ModuleProxy).Warn(msg, fields...)
}

func logProxyInfo(msg string, fields ...logging.Field) {
	logging.Default().For(logging.ModuleProxy).Info(msg, fields...)
}

func logProxyError(msg string, fields ...logging.Field) {
	logging.Default().For(logging.ModuleProxy).Error(msg, fields...)
}

func logChatEntry(reqID, httpReqID, model string, stream bool, msgsCount, toolsCount int, session, run string) {
	logProxyInfo("chat request received",
		logging.F("req", reqID),
		logging.F("http_req", httpReqID),
		logging.F("model", model),
		logging.F("stream", stream),
		logging.F("messages", msgsCount),
		logging.F("tools", toolsCount),
		logging.F("session", session),
		logging.F("run", run),
	)
}

func logChatAuthFail() {
	logProxyWarn("proxy auth failed", logging.F("status", 401))
}

func logChatParseFail(reason string) {
	logProxyWarn("invalid request", logging.F("status", 400), logging.F("reason", reason))
}

func logAccountSelected(reqID, provider, account string, count int) {
	logProxyInfo("account selected",
		logging.F("req", reqID),
		logging.F("provider", provider),
		logging.F("account", account),
		logging.F("candidates", count),
	)
}

func logCredError(account, reason string, err error) {
	logProxyError("credential error",
		logging.F("account", account),
		logging.F("reason", reason),
		logging.F("error", logging.Redact(err.Error(), 200)),
	)
}

func logCredRefresh(account string, success bool) {
	status := "ok"
	if !success {
		status = "failed"
	}
	logProxyInfo("credential refresh", logging.F("account", account), logging.F("refresh", status))
}

func logUpstreamStart(reqID, provider, model, account string) {
	logProxyInfo("upstream call started",
		logging.F("req", reqID),
		logging.F("provider", provider),
		logging.F("model", model),
		logging.F("account", account),
	)
}

func logUpstreamEnd(reqID string, status int, durationMs, ttftMs int64, err error) {
	fields := []logging.Field{
		logging.F("req", reqID),
		logging.F("status", status),
		logging.F("duration_ms", durationMs),
	}
	if ttftMs > 0 {
		fields = append(fields, logging.F("ttft_ms", ttftMs))
	}
	if err != nil {
		fields = append(fields, logging.F("error", logging.Redact(err.Error(), 200)))
		logProxyWarn("upstream call failed", fields...)
	} else {
		logProxyInfo("upstream call finished", fields...)
	}
}
