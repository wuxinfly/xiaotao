package app

import (
	"encoding/json"
	"io"
	"net/http"

	"xiaotao/xt-client/internal/agent"
)

func registerSkillRoutes(mux *http.ServeMux, svc *agent.Service, requireOrigin func(*http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	for _, method := range []string{"GET", "PUT", "POST"} {
		path := "/api/agent/skills"
		if method == "POST" {
			path += "/scan"
		}
		mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
			if !requireOrigin(r) {
				http.Error(w, "same-origin required", 403)
				return
			}
			if r.Method == "GET" {
				catalog, err := svc.LocalSkills()
				if err != nil {
					apiError(w, 500, err.Error())
					return
				}
				send(w, catalog)
				return
			}
			var settings agent.SkillsSettings
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&settings); err != nil || decoder.Decode(new(any)) != io.EOF {
				apiError(w, 400, "Skills 配置格式无效")
				return
			}
			catalog, err := agent.ScanLocalSkills(settings)
			if err != nil {
				apiError(w, 400, err.Error())
				return
			}
			if r.Method == "PUT" {
				if err = svc.SaveSkillsSettings(catalog.Settings); err != nil {
					apiError(w, 400, err.Error())
					return
				}
			}
			send(w, catalog)
		})
	}
}
