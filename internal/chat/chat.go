// Package chat is players writing to each other (net.chats, chat_members, chat_messages,
// chat_blocks; migration 0006).
//
// A chat is between two players and there is one for each two: opening a chat with someone
// gives the one they already have. Messages are numbered in the order written over all chats
// (seq), so "everything after n" is one number for all of a player's chats, and each member
// has read their chat up to a number of their own (read_seq). A player may block another;
// while either has blocked the other, neither can write in their chat.
//
// The site's own notices are not chats: each song source keeps them (notifications). The
// website shows them beside the chats as one of their own.
package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// MaxBody is the longest message, in characters.
	MaxBody = 2000
	// MaxPage is the most messages one read gives.
	MaxPage = 100
	// MaxChats is the most chats a list gives, the latest written first.
	MaxChats = 200
)

var (
	ErrSelf     = errors.New("chat: a chat with oneself")
	ErrNoPlayer = errors.New("chat: no such player")
	ErrNotFound = errors.New("chat: no such chat of the player's")
	ErrBlocked  = errors.New("chat: one of the two has blocked the other")
	ErrBody     = errors.New("chat: the message is empty or too long")
)

// Message is one message of a chat.
type Message struct {
	Seq       int64     `json:"seq"`
	ChatID    string    `json:"chatId"`
	SenderID  string    `json:"senderId"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Chat is a chat as one of its members sees it. PeerID is the other member; Unread how many
// of the peer's messages come after ReadSeq; PeerReadSeq how far the peer has read. Blocked:
// this member has blocked the peer. CanWrite: neither has blocked the other.
type Chat struct {
	ID          string    `json:"id"`
	PeerID      string    `json:"-"`
	LastMessage *Message  `json:"lastMessage"`
	Unread      int       `json:"unread"`
	ReadSeq     int64     `json:"readSeq"`
	PeerReadSeq int64     `json:"peerReadSeq"`
	Blocked     bool      `json:"blocked"`
	CanWrite    bool      `json:"canWrite"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Store keeps chats in ourtaiko_net.
type Store struct {
	DB *pgxpool.Pool
}

// pair names the chat of two players.
func pair(a, b string) string {
	if b < a {
		a, b = b, a
	}
	return a + ":" + b
}

// Body is a message as it is kept: without the blank around it. ErrBody: nothing is left, or
// more than MaxBody characters, or not text.
func Body(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxBody || strings.ContainsRune(text, 0) {
		return "", ErrBody
	}
	return text, nil
}

// One chat as me sees it; $1 is me. c is the chat, m me as its member, p the peer as one.
const chatColumns = `c.id, p.user_id, c.created_at, m.read_seq, p.read_seq,
 EXISTS (SELECT 1 FROM net.chat_blocks b WHERE b.user_id = $1 AND b.blocked_id = p.user_id),
 NOT EXISTS (SELECT 1 FROM net.chat_blocks b WHERE (b.user_id = $1 AND b.blocked_id = p.user_id) OR (b.user_id = p.user_id AND b.blocked_id = $1)),
 (SELECT count(*) FROM net.chat_messages x WHERE x.chat_id = c.id AND x.seq > m.read_seq AND x.sender_id <> $1),
 l.seq, l.sender_id, l.body, l.created_at
 FROM net.chats c
 JOIN net.chat_members m ON m.chat_id = c.id AND m.user_id = $1
 JOIN net.chat_members p ON p.chat_id = c.id AND p.user_id <> $1
 LEFT JOIN LATERAL (SELECT seq, sender_id, body, created_at FROM net.chat_messages x WHERE x.chat_id = c.id ORDER BY seq DESC LIMIT 1) l ON true`

func scanChat(row pgx.Row) (Chat, error) {
	var c Chat
	var seq *int64
	var sender, body *string
	var at *time.Time
	if err := row.Scan(&c.ID, &c.PeerID, &c.CreatedAt, &c.ReadSeq, &c.PeerReadSeq, &c.Blocked, &c.CanWrite, &c.Unread, &seq, &sender, &body, &at); err != nil {
		return c, err
	}
	if seq != nil {
		c.LastMessage = &Message{Seq: *seq, ChatID: c.ID, SenderID: *sender, Body: *body, CreatedAt: *at}
	}
	return c, nil
}

// Get is the chat id as me sees it. ErrNotFound: there is none, or me is not in it.
func (s Store) Get(ctx context.Context, me, id string) (Chat, error) {
	c, err := scanChat(s.DB.QueryRow(ctx, `SELECT `+chatColumns+` WHERE c.id = $2`, me, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Open is the chat of me and peer, made if they have none. ErrSelf, ErrNoPlayer: peer is me,
// or has never been here.
func (s Store) Open(ctx context.Context, me, peer, id string) (Chat, error) {
	if me == peer {
		return Chat{}, ErrSelf
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Chat{}, err
	}
	defer tx.Rollback(ctx)
	var players int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM net.users WHERE id = ANY($1)`, []string{me, peer}).Scan(&players); err != nil {
		return Chat{}, err
	}
	if players != 2 {
		return Chat{}, ErrNoPlayer
	}
	// A chat made by another request meanwhile is the one.
	if err = tx.QueryRow(ctx, `WITH made AS (INSERT INTO net.chats (id, pair) VALUES ($1, $2) ON CONFLICT (pair) DO NOTHING RETURNING id)
SELECT id FROM made UNION ALL SELECT id FROM net.chats WHERE pair = $2`, id, pair(me, peer)).Scan(&id); err != nil {
		return Chat{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO net.chat_members (chat_id, user_id) VALUES ($1, $2), ($1, $3) ON CONFLICT DO NOTHING`, id, me, peer); err != nil {
		return Chat{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Chat{}, err
	}
	return s.Get(ctx, me, id)
}

// List is me's chats that have been written in, the latest written first (at most MaxChats),
// and the number of the latest message in any of me's chats (0: none).
func (s Store) List(ctx context.Context, me string) ([]Chat, int64, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+chatColumns+` WHERE c.last_message_at IS NOT NULL ORDER BY c.last_message_at DESC, c.id LIMIT $2`, me, MaxChats)
	if err != nil {
		return nil, 0, err
	}
	chats, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Chat, error) { return scanChat(row) })
	if err != nil {
		return nil, 0, err
	}
	cursor, err := s.Cursor(ctx, me)
	return chats, cursor, err
}

// Cursor is the number of the latest message in any of me's chats (0: none).
func (s Store) Cursor(ctx context.Context, me string) (int64, error) {
	var cursor int64
	err := s.DB.QueryRow(ctx, `SELECT COALESCE(max(x.seq), 0) FROM net.chat_messages x JOIN net.chat_members m ON m.chat_id = x.chat_id AND m.user_id = $1`, me).Scan(&cursor)
	return cursor, err
}

// Messages is a page of the chat id, oldest first: the limit (1 to MaxPage) latest before the
// message before (0: the latest of all), and whether there are older ones.
func (s Store) Messages(ctx context.Context, me, id string, before int64, limit int) ([]Message, bool, error) {
	if _, err := s.Get(ctx, me, id); err != nil {
		return nil, false, err
	}
	if limit < 1 || limit > MaxPage {
		limit = MaxPage
	}
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.DB.Query(ctx, `SELECT seq, chat_id, sender_id, body, created_at FROM net.chat_messages
 WHERE chat_id = $1 AND seq < $2 ORDER BY seq DESC LIMIT $3`, id, before, limit+1)
	if err != nil {
		return nil, false, err
	}
	page, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Message])
	if err != nil {
		return nil, false, err
	}
	more := len(page) > limit
	if more {
		page = page[:limit]
	}
	for i, j := 0, len(page)-1; i < j; i, j = i+1, j-1 {
		page[i], page[j] = page[j], page[i]
	}
	return page, more, nil
}

// Send writes body (as Body keeps it) from me in the chat id, which me has then read up to
// it. It gives the message and the peer.
func (s Store) Send(ctx context.Context, me, id, body string) (Message, string, error) {
	body, err := Body(body)
	if err != nil {
		return Message{}, "", err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Message{}, "", err
	}
	defer tx.Rollback(ctx)
	// The chat's row is locked so that a block made meanwhile is either before or after.
	var peer string
	var open bool
	err = tx.QueryRow(ctx, `SELECT p.user_id,
 NOT EXISTS (SELECT 1 FROM net.chat_blocks b WHERE (b.user_id = $1 AND b.blocked_id = p.user_id) OR (b.user_id = p.user_id AND b.blocked_id = $1))
 FROM net.chats c
 JOIN net.chat_members m ON m.chat_id = c.id AND m.user_id = $1
 JOIN net.chat_members p ON p.chat_id = c.id AND p.user_id <> $1
 WHERE c.id = $2 FOR UPDATE OF c`, me, id).Scan(&peer, &open)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, "", ErrNotFound
	}
	if err != nil {
		return Message{}, "", err
	}
	if !open {
		return Message{}, peer, ErrBlocked
	}
	m := Message{ChatID: id, SenderID: me, Body: body}
	if err = tx.QueryRow(ctx, `INSERT INTO net.chat_messages (chat_id, sender_id, body) VALUES ($1, $2, $3) RETURNING seq, created_at`, id, me, body).Scan(&m.Seq, &m.CreatedAt); err != nil {
		return Message{}, "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE net.chats SET last_message_at = $2 WHERE id = $1`, id, m.CreatedAt); err != nil {
		return Message{}, "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE net.chat_members SET read_seq = $3 WHERE chat_id = $1 AND user_id = $2`, id, me, m.Seq); err != nil {
		return Message{}, "", err
	}
	return m, peer, tx.Commit(ctx)
}

// Read marks the chat id read by me up to the message seq (no further than its latest). It
// gives the peer, and whether anything changed.
func (s Store) Read(ctx context.Context, me, id string, seq int64) (string, bool, error) {
	c, err := s.Get(ctx, me, id)
	if err != nil {
		return "", false, err
	}
	tag, err := s.DB.Exec(ctx, `UPDATE net.chat_members m
 SET read_seq = LEAST($3, (SELECT COALESCE(max(seq), 0) FROM net.chat_messages x WHERE x.chat_id = $1))
 WHERE m.chat_id = $1 AND m.user_id = $2
   AND m.read_seq < LEAST($3, (SELECT COALESCE(max(seq), 0) FROM net.chat_messages x WHERE x.chat_id = $1))`, id, me, seq)
	if err != nil {
		return "", false, err
	}
	return c.PeerID, tag.RowsAffected() > 0, nil
}

// Unread is how many messages me has not read, and in how many chats.
func (s Store) Unread(ctx context.Context, me string) (messages, chats int, err error) {
	err = s.DB.QueryRow(ctx, `SELECT count(*), count(DISTINCT x.chat_id) FROM net.chat_members m
 JOIN net.chat_messages x ON x.chat_id = m.chat_id AND x.seq > m.read_seq AND x.sender_id <> $1
 WHERE m.user_id = $1`, me).Scan(&messages, &chats)
	return
}

// Since is what has been written in me's chats after the message after, oldest first, at
// most limit (1 to MaxPage) of it.
func (s Store) Since(ctx context.Context, me string, after int64, limit int) ([]Message, error) {
	if limit < 1 || limit > MaxPage {
		limit = MaxPage
	}
	rows, err := s.DB.Query(ctx, `SELECT x.seq, x.chat_id, x.sender_id, x.body, x.created_at FROM net.chat_messages x
 JOIN net.chat_members m ON m.chat_id = x.chat_id AND m.user_id = $1
 WHERE x.seq > $2 ORDER BY x.seq LIMIT $3`, me, after, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Message])
}

// Block blocks or unblocks the peer of the chat id for me, and gives the chat as it is then.
func (s Store) Block(ctx context.Context, me, id string, block bool) (Chat, error) {
	c, err := s.Get(ctx, me, id)
	if err != nil {
		return c, err
	}
	if block {
		_, err = s.DB.Exec(ctx, `INSERT INTO net.chat_blocks (user_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, me, c.PeerID)
	} else {
		_, err = s.DB.Exec(ctx, `DELETE FROM net.chat_blocks WHERE user_id = $1 AND blocked_id = $2`, me, c.PeerID)
	}
	if err != nil {
		return c, err
	}
	return s.Get(ctx, me, id)
}

// Hub wakes those who wait for something new in their chats. It knows only this process's
// waiters: the site runs as one.
type Hub struct {
	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

// Wait is a channel that is closed when user is woken, and what to call when no longer
// waiting. Wait before reading what is there, so that nothing written in between is missed.
func (h *Hub) Wait(user string) (<-chan struct{}, func()) {
	ch := make(chan struct{})
	h.mu.Lock()
	if h.waiters == nil {
		h.waiters = map[string]map[chan struct{}]struct{}{}
	}
	if h.waiters[user] == nil {
		h.waiters[user] = map[chan struct{}]struct{}{}
	}
	h.waiters[user][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if set, ok := h.waiters[user]; ok {
			if _, ok := set[ch]; ok {
				delete(set, ch)
				close(ch)
			}
			if len(set) == 0 {
				delete(h.waiters, user)
			}
		}
	}
}

// Wake wakes everyone waiting of users.
func (h *Hub) Wake(users ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, user := range users {
		for ch := range h.waiters[user] {
			close(ch)
		}
		delete(h.waiters, user)
	}
}
