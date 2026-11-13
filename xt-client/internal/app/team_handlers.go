package app

import (
	"encoding/json"
	"errors"
	"net/http"

	"xiaotao/xt-client/internal/office"
)

type teamWriteFunc func() (any, error)

func registerTeamRoutes(mux *http.ServeMux, svc *office.Service, guard func(http.ResponseWriter, *http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	write := func(w http.ResponseWriter, r *http.Request, target any, action teamWriteFunc) {
		if !guard(w, r) {
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			apiError(w, 400, "无效的请求: "+err.Error())
			return
		}
		result, err := action()
		switch {
		case errors.Is(err, office.ErrRoleConflict), errors.Is(err, office.ErrMemberActive):
			apiError(w, http.StatusConflict, err.Error())
		case errors.Is(err, office.ErrRoleCorrupt):
			apiError(w, http.StatusUnprocessableEntity, err.Error())
		case err != nil:
			apiError(w, http.StatusBadRequest, err.Error())
		default:
			send(w, result)
		}
	}
	mux.HandleFunc("GET /api/office/team/templates", func(w http.ResponseWriter, r *http.Request) {
		if guard(w, r) {
			send(w, svc.TeamTemplates())
		}
	})
	mux.HandleFunc("GET /api/office/team", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		result, err := svc.TeamSnapshot(r.URL.Query().Get("projectPath"))
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		send(w, result)
	})
	mux.HandleFunc("POST /api/office/team/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		var request office.TeamRequest
		write(w, r, &request, func() (any, error) { return svc.BootstrapTeam(request) })
	})
	mux.HandleFunc("POST /api/office/team/skip-onboarding", func(w http.ResponseWriter, r *http.Request) {
		var request office.TeamRequest
		write(w, r, &request, func() (any, error) { return svc.SkipTeamOnboarding(request) })
	})
	roleAction := func(action func(office.RoleRequest) (office.TeamSnapshot, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var request office.RoleRequest
			write(w, r, &request, func() (any, error) { return action(request) })
		}
	}
	mux.HandleFunc("POST /api/office/roles", roleAction(svc.CreateTeamRole))
	mux.HandleFunc("PUT /api/office/roles", roleAction(svc.UpdateTeamRole))
	mux.HandleFunc("POST /api/office/roles/invite", roleAction(svc.InviteTeamRole))
	mux.HandleFunc("POST /api/office/roles/remove", roleAction(svc.RemoveTeamRole))
	mux.HandleFunc("POST /api/office/roles/promote", roleAction(svc.PromoteTeamRole))
	mux.HandleFunc("POST /api/office/roles/archive", roleAction(svc.ArchiveTeamRole))
}
