package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"ourtaiko.dev/net/internal/chat"
	"ourtaiko.dev/net/internal/web"
)

// Players writing to each other (package chat), all of it for the signed-in player only:
//
//	GET    /api/v1/chats                      their chats, the latest written first, and a cursor
//	POST   /api/v1/chats {"userId"}           the chat with a player, made if there is none
//	GET    /api/v1/chats/unread               how many messages are unread, in how many chats,
//	                                          and the cursor to wait for updates from
//	GET    /api/v1/chats/updates?after=<seq>  messages after the cursor; waits for some (below)
//	GET    /api/v1/chats/{id}                 one chat
//	GET    /api/v1/chats/{id}/messages?before=<seq>&limit=<n>
//	POST   /api/v1/chats/{id}/messages {"body"}
//	POST   /api/v1/chats/{id}/read {"seq"}    read up to a message
//	PUT    /api/v1/chats/{id}/block {"blocked"}
//
// updates is a long poll: when nothing is newer than after, it waits up to chatWait for a
// message or for a peer to read, then answers {"messages": [...], "cursor": n, "changed": b}.
// changed: something happened in the player's chats (a read, a block) besides the messages.

// How long an updates request waits for something to happen.
var chatWait = 25 * time.Second

// ChatPerson is a member of a chat as the site shows them.
type ChatPerson struct {
	ID        string `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

// ChatView is a chat with its peer shown.
type ChatView struct {
	chat.Chat
	Peer ChatPerson `json:"peer"`
}

func (s *Server) chatStore() chat.Store { return chat.Store{DB: s.DB} }

// chatViews names the peers of chats.
func (s *Server) chatViews(ctx context.Context, chats ...chat.Chat) []ChatView {
	ids := make([]string, 0, len(chats))
	for _, c := range chats {
		ids = append(ids, c.PeerID)
	}
	names := s.profiles(ctx, ids)
	out := make([]ChatView, len(chats))
	for i, c := range chats {
		p := names[c.PeerID]
		out[i] = ChatView{Chat: c, Peer: ChatPerson{ID: c.PeerID, Nickname: p.Nickname, AvatarURL: p.AvatarURL}}
	}
	return out
}

// chatProblem answers an error of package chat.
func chatProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, chat.ErrNotFound):
		problem(w, 404, "CHAT_NOT_FOUND", "聊天不存在")
	case errors.Is(err, chat.ErrSelf):
		problem(w, 400, "CHAT_SELF", "不能和自己聊天")
	case errors.Is(err, chat.ErrNoPlayer):
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
	case errors.Is(err, chat.ErrBlocked):
		problem(w, 403, "CHAT_BLOCKED", "无法向对方发送消息")
	case errors.Is(err, chat.ErrBody):
		problem(w, 400, "MESSAGE_INVALID", "消息不能为空，最多 2000 字")
	default:
		internal(w, err)
	}
}

func (s *Server) listChats(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	chats, cursor, err := s.chatStore().List(r.Context(), u.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"items": s.chatViews(r.Context(), chats...), "cursor": cursor})
}

func (s *Server) openChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"userId"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := s.chatStore().Open(r.Context(), u.User.ID, body.UserID, web.NewID())
	if err != nil {
		chatProblem(w, err)
		return
	}
	web.Respond(w, 200, s.chatViews(r.Context(), c)[0])
}

func (s *Server) getChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	c, err := s.chatStore().Get(r.Context(), u.User.ID, r.PathValue("id"))
	if err != nil {
		chatProblem(w, err)
		return
	}
	web.Respond(w, 200, s.chatViews(r.Context(), c)[0])
}

// seqQuery reads a message number from the query; 0 when absent.
func seqQuery(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	text := r.URL.Query().Get(name)
	if text == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < 0 {
		problem(w, 400, "QUERY_INVALID", name+" 需要是非负整数")
		return 0, false
	}
	return n, true
}

func (s *Server) chatMessages(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	before, ok := seqQuery(w, r, "before")
	if !ok {
		return
	}
	limit := 50
	if text := r.URL.Query().Get("limit"); text != "" {
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > chat.MaxPage {
			problem(w, 400, "QUERY_INVALID", "limit 需要是 1 到 100 的整数")
			return
		}
		limit = n
	}
	items, more, err := s.chatStore().Messages(r.Context(), u.User.ID, r.PathValue("id"), before, limit)
	if err != nil {
		chatProblem(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items, "more": more})
}

func (s *Server) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if !answers.DecodeLimit(w, r, &body, 16*1024) {
		return
	}
	m, peer, err := s.chatStore().Send(r.Context(), u.User.ID, r.PathValue("id"), body.Body)
	if err != nil {
		chatProblem(w, err)
		return
	}
	s.chats.Wake(u.User.ID, peer)
	web.Respond(w, 201, m)
}

func (s *Server) readChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Seq int64 `json:"seq"`
	}
	if !decode(w, r, &body) {
		return
	}
	peer, changed, err := s.chatStore().Read(r.Context(), u.User.ID, r.PathValue("id"), body.Seq)
	if err != nil {
		chatProblem(w, err)
		return
	}
	// The peer sees how far they were read; the reader's other pages, their count.
	if changed {
		s.chats.Wake(u.User.ID, peer)
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

func (s *Server) blockChat(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Blocked *bool `json:"blocked"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Blocked == nil {
		problem(w, 400, "REQUEST_INVALID", "缺少 blocked")
		return
	}
	c, err := s.chatStore().Block(r.Context(), u.User.ID, r.PathValue("id"), *body.Blocked)
	if err != nil {
		chatProblem(w, err)
		return
	}
	s.chats.Wake(u.User.ID, c.PeerID)
	web.Respond(w, 200, s.chatViews(r.Context(), c)[0])
}

func (s *Server) unreadChats(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	store := s.chatStore()
	messages, chats, err := store.Unread(r.Context(), u.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	cursor, err := store.Cursor(r.Context(), u.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]int64{"count": int64(messages), "chats": int64(chats), "cursor": cursor})
}

func (s *Server) chatUpdates(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	after, ok := seqQuery(w, r, "after")
	if !ok {
		return
	}
	store, me := s.chatStore(), u.User.ID
	answer := func(changed bool) {
		items, err := store.Since(r.Context(), me, after, chat.MaxPage)
		if err != nil {
			internal(w, err)
			return
		}
		cursor := after
		if len(items) > 0 {
			cursor = items[len(items)-1].Seq
		}
		web.Respond(w, 200, map[string]any{"messages": items, "cursor": cursor, "changed": changed})
	}
	// Waiting starts before looking, so that nothing written in between is missed.
	woken, stop := s.chats.Wait(me)
	defer stop()
	latest, err := store.Cursor(r.Context(), me)
	if err != nil {
		internal(w, err)
		return
	}
	if latest > after || r.URL.Query().Get("wait") == "0" {
		answer(false)
		return
	}
	timer := time.NewTimer(chatWait)
	defer timer.Stop()
	select {
	case <-woken:
		answer(true)
	case <-timer.C:
		answer(false)
	case <-r.Context().Done():
	}
}
