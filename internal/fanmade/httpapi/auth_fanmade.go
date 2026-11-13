package httpapi

import (
	"errors"
	"net/http"
)

// What only the fanmade source's sign-in has: the password login it still passes on to the
// account center for older games, and the answer to account routes it gave up when accounts
// moved there.

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	var result struct {
		User    User   `json:"user"`
		Token   string `json:"accessToken"`
		Expires int    `json:"expiresIn"`
	}
	e := s.Config.SSO.Call(r.Context(), "game/login", c, &result)
	var remote *ssoError
	if errors.As(e, &remote) {
		switch remote.Status {
		case 401:
			problem(w, 401, "LOGIN_INVALID", "用户名或密码不正确")
			return
		case 429:
			w.Header().Set("Retry-After", "600")
			problem(w, 429, "RATE_LIMITED", "尝试次数过多，请稍后重试")
			return
		case 400:
			problem(w, 400, "REQUEST_INVALID", "登录信息格式不正确")
			return
		}
	}
	if e != nil {
		internal(w, e)
		return
	}
	if !validSubject(result.User.ID) || len(result.Token) != 64 {
		internal(w, errors.New("invalid SSO login response"))
		return
	}
	if e = recordUserActivity(r.Context(), s.DB, result.User.ID, true); e != nil {
		internal(w, e)
		return
	}
	result.User.AvatarURL = s.Config.SSO.AvatarURL(result.User)
	respond(w, 200, result)
}

func (s *Server) retiredAccountAPI(w http.ResponseWriter, r *http.Request) {
	problem(w, 410, "SSO_REQUIRED", "账号管理已迁至 OurTaiko 账号中心，请使用 SSO 登录或管理账号")
}
