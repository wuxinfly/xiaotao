package app

import (
	"encoding/json"
	"errors"
	"net/http"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/office"
)

func registerOfficeConversationRoutes(mux *http.ServeMux, svc *office.Service, guard func(http.ResponseWriter, *http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	base := "/api/office/conversations/{parent}"
	mux.HandleFunc("GET "+base+"/members/{member}/events", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		events, unsubscribe, err := svc.SubscribeConversation(r.PathValue("parent"), r.URL.Query().Get("projectPath"), r.PathValue("member"))
		if err != nil {
			officeError(w, err, apiError)
			return
		}
		defer unsubscribe()
		streamOfficeEvents(w, r, events, apiError)
	})
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		result, err := svc.Conversations(r.PathValue("parent"))
		sendOfficeConversationResult(w, result, err, send, apiError)
	})
	mux.HandleFunc("POST "+base+"/dispatch", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var input office.DispatchInput
		if !readOfficeConversationInput(w, r, &input, apiError) {
			return
		}
		result, err := svc.DispatchConversation(r.PathValue("parent"), input)
		sendOfficeConversationResult(w, result, err, send, apiError)
	})
	mux.HandleFunc("POST "+base+"/reply", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var input office.ReplyInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&input) != nil {
			apiError(w, 400, "员工回复格式无效")
			return
		}
		result, err := svc.ReplyConversation(r.Context(), r.PathValue("parent"), input)
		sendOfficeConversationResult(w, result, err, send, apiError)
	})
	mux.HandleFunc("POST "+base+"/retry-summary", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var input struct {
			MemberID string `json:"memberId"`
		}
		if !readOfficeConversationInput(w, r, &input, apiError) {
			return
		}
		result, err := svc.RetryConversationSummary(r.PathValue("parent"), input.MemberID)
		sendOfficeConversationResult(w, result, err, send, apiError)
	})
	mux.HandleFunc("POST "+base+"/close", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var input office.CloseConversationInput
		if !readOfficeConversationInput(w, r, &input, apiError) {
			return
		}
		err := svc.CloseConversations(r.Context(), r.PathValue("parent"), input)
		sendOfficeConversationResult(w, map[string]bool{"ok": true}, err, send, apiError)
	})
}

func readOfficeConversationInput(w http.ResponseWriter, r *http.Request, input any, apiError func(http.ResponseWriter, int, string)) bool {
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(input) == nil {
		return true
	}
	apiError(w, 400, "员工对话请求格式无效")
	return false
}

func sendOfficeConversationResult(w http.ResponseWriter, result any, err error, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	if err == nil {
		send(w, result)
		return
	}
	code := ""
	switch {
	case errors.Is(err, office.ErrConversationBusy):
		code = "active"
	case errors.Is(err, office.ErrSummaryPending):
		code = "summary_pending"
	case errors.Is(err, office.ErrSummaryFailed):
		code = "summary_failed"
	}
	if code == "" {
		officeError(w, err, apiError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	send(w, map[string]any{"error": map[string]string{"message": err.Error(), "code": code}})
}

func deleteOfficeRootSession(w http.ResponseWriter, r *http.Request, agentSvc *agent.Service, officeSvc *office.Service, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	id := r.PathValue("id")
	session, err := agentSvc.OfficeSessionMetadata(id)
	if err != nil {
		apiError(w, 404, "会话不存在")
		return
	}
	if session.Role != "" || session.ReadOnly {
		apiError(w, 400, "员工对话随小涛主会话清理")
		return
	}
	query := r.URL.Query()
	input := office.CloseConversationInput{Force: query.Get("force") == "true", DiscardSummary: query.Get("discardSummary") == "true"}
	err = agentSvc.DeleteOfficeRoot(id, func(commit func() error) error {
		return officeSvc.CloseConversations(r.Context(), id, input, commit)
	})
	sendOfficeConversationResult(w, map[string]bool{"ok": true}, err, send, apiError)
}
