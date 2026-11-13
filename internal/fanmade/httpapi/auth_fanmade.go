package httpapi

import (
	"net/http"
)

// What only the fanmade source's sign-in has: the answer to account routes it gave up when
// accounts moved to the account center.

func (s *Server) retiredAccountAPI(w http.ResponseWriter, r *http.Request) {
	problem(w, 410, "SSO_REQUIRED", "账号管理已迁至 OurTaiko 账号中心，请使用 SSO 登录或管理账号")
}
