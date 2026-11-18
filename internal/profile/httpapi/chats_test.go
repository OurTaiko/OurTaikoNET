package httpapi

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/chat"
)

// signInAs opens a development session for the server's own player, id.
func (c *client) signInAs(id string) {
	c.t.Helper()
	w := c.do("GET", "/api/auth/sso/login?returnTo=/", "", nil)
	if w.Code != 303 || len(w.Result().Cookies()) != 1 {
		c.t.Fatal("dev sign-in", w.Code)
	}
	c.cookie = w.Result().Cookies()[0].Value
	var me struct {
		User      *User
		CSRFToken string
	}
	c.json("GET", "/api/me", "", 200, &me)
	if me.User == nil || me.User.ID != id {
		c.t.Fatal("not signed in", me)
	}
	c.csrf = me.CSRFToken
}

type chatList struct {
	Items  []ChatView
	Cursor int64
}
type chatUpdate struct {
	Messages []chat.Message
	Cursor   int64
	Changed  bool
}

func TestPlayersWriteToEachOther(t *testing.T) {
	pool := testDB(t)
	// Two players on one database, each signed in on a server of their own that hears the other.
	alice := New(pool, Config{Origin: origin, DevUserID: player})
	bob := New(pool, Config{Origin: origin, DevUserID: other})
	bob.chats = alice.chats
	a := &client{t: t, handler: alice.Handler()}
	b := &client{t: t, handler: bob.Handler()}

	a.json("GET", "/api/chats", "", 401, nil)
	a.signInAs(player)
	// Someone who has never been here cannot be written to, nor can oneself.
	a.json("POST", "/api/chats", `{"userId":"`+other+`"}`, 404, nil)
	a.json("POST", "/api/chats", `{"userId":"`+player+`"}`, 400, nil)
	b.signInAs(other)

	var list chatList
	a.json("GET", "/api/chats", "", 200, &list)
	if len(list.Items) != 0 || list.Cursor != 0 {
		t.Fatalf("before anything %+v", list)
	}
	var opened, again ChatView
	a.json("POST", "/api/chats", `{"userId":"`+other+`"}`, 200, &opened)
	b.json("POST", "/api/chats", `{"userId":"`+player+`"}`, 200, &again)
	if opened.ID == "" || again.ID != opened.ID || opened.Peer.ID != other || again.Peer.ID != player || !opened.CanWrite || opened.LastMessage != nil {
		t.Fatalf("one chat for two: %+v %+v", opened, again)
	}
	// A chat nothing has been written in is not listed.
	a.json("GET", "/api/chats", "", 200, &list)
	if len(list.Items) != 0 {
		t.Fatalf("an empty chat listed %+v", list)
	}
	path := "/api/chats/" + opened.ID

	// Bob waits for news; Alice writes.
	waited := make(chan chatUpdate, 1)
	go func() {
		var u chatUpdate
		b.json("GET", "/api/chats/updates?after=0", "", 200, &u)
		waited <- u
	}()
	time.Sleep(100 * time.Millisecond)
	a.json("POST", path+"/messages", `{"body":"  "}`, 400, nil)
	a.json("POST", path+"/messages", `{"body":"`+strings.Repeat("咚", chat.MaxBody+1)+`"}`, 400, nil)
	var first chat.Message
	a.json("POST", path+"/messages", `{"body":"  hello, Bob\n"}`, 201, &first)
	if first.Body != "hello, Bob" || first.SenderID != player || first.ChatID != opened.ID {
		t.Fatalf("sent %+v", first)
	}
	select {
	case u := <-waited:
		if len(u.Messages) != 1 || u.Messages[0].Seq != first.Seq || u.Cursor != first.Seq {
			t.Fatalf("woken with %+v", u)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Bob was not woken")
	}

	var count struct {
		Count, Chats int
		Cursor       int64
	}
	b.json("GET", "/api/chats/unread", "", 200, &count)
	if count.Count != 1 || count.Chats != 1 || count.Cursor != first.Seq {
		t.Fatalf("Bob's unread %+v", count)
	}
	a.json("GET", "/api/chats/unread", "", 200, &count)
	if count.Count != 0 {
		t.Fatalf("one's own message unread %+v", count)
	}
	b.json("GET", "/api/chats", "", 200, &list)
	if len(list.Items) != 1 || list.Items[0].Unread != 1 || list.Items[0].LastMessage == nil || list.Items[0].LastMessage.Body != "hello, Bob" || list.Cursor != first.Seq {
		t.Fatalf("Bob's chats %+v", list)
	}

	// Bob answers twice, then reads; Alice sees how far.
	var second, third chat.Message
	b.json("POST", path+"/messages", `{"body":"hi"}`, 201, &second)
	b.json("POST", path+"/messages", `{"body":"how are you"}`, 201, &third)
	b.json("POST", path+"/read", `{"seq":999999}`, 200, nil)
	var seen ChatView
	a.json("GET", path, "", 200, &seen)
	if seen.Unread != 2 || seen.PeerReadSeq != third.Seq || seen.LastMessage.Seq != third.Seq {
		t.Fatalf("Alice's view %+v", seen)
	}
	var page struct {
		Items []chat.Message
		More  bool
	}
	a.json("GET", path+"/messages?limit=2", "", 200, &page)
	if len(page.Items) != 2 || !page.More || page.Items[0].Seq != second.Seq || page.Items[1].Seq != third.Seq {
		t.Fatalf("latest page %+v", page)
	}
	a.json("GET", path+"/messages?limit=2&before="+itoa(second.Seq), "", 200, &page)
	if len(page.Items) != 1 || page.More || page.Items[0].Seq != first.Seq {
		t.Fatalf("older page %+v", page)
	}
	a.json("GET", path+"/messages?limit=0", "", 400, nil)
	var u chatUpdate
	a.json("GET", "/api/chats/updates?after="+itoa(first.Seq), "", 200, &u)
	if len(u.Messages) != 2 || u.Cursor != third.Seq {
		t.Fatalf("updates %+v", u)
	}
	a.json("POST", path+"/read", `{"seq":`+itoa(second.Seq)+`}`, 200, nil)
	a.json("GET", "/api/chats/unread", "", 200, &count)
	if count.Count != 1 {
		t.Fatalf("read up to the second %+v", count)
	}

	// A third player sees nothing of it.
	carol := New(pool, Config{Origin: origin, DevUserID: "0000000000000000000000000000ca01"})
	c := &client{t: t, handler: carol.Handler()}
	c.signInAs("0000000000000000000000000000ca01")
	c.json("GET", path, "", 404, nil)
	c.json("GET", path+"/messages", "", 404, nil)
	c.json("POST", path+"/messages", `{"body":"hey"}`, 404, nil)
	c.json("GET", "/api/chats/updates?after=0&wait=0", "", 200, &u)
	if len(u.Messages) != 0 {
		t.Fatalf("a stranger's updates %+v", u)
	}

	// Blocked, neither can write; unblocked, both can.
	var blocked ChatView
	a.json("PUT", path+"/block", `{"blocked":true}`, 200, &blocked)
	if !blocked.Blocked || blocked.CanWrite {
		t.Fatalf("blocked %+v", blocked)
	}
	b.json("GET", path, "", 200, &seen)
	if seen.Blocked || seen.CanWrite {
		t.Fatalf("the blocked one's view %+v", seen)
	}
	b.json("POST", path+"/messages", `{"body":"?"}`, 403, nil)
	a.json("POST", path+"/messages", `{"body":"!"}`, 403, nil)
	a.json("PUT", path+"/block", `{"blocked":false}`, 200, &blocked)
	b.json("POST", path+"/messages", `{"body":"back"}`, 201, nil)

	// Reading is said each time a message arrives: it costs none of the player's writes.
	for range 45 {
		a.json("POST", path+"/read", `{"seq":0}`, 200, nil)
	}

	// Without the write's token, nothing is written.
	csrf := a.csrf
	a.csrf = "wrong"
	a.json("POST", path+"/messages", `{"body":"x"}`, 403, nil)
	a.csrf = csrf
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestChatUpdatesWaitAndGiveUp(t *testing.T) {
	saved := chatWait
	chatWait = 50 * time.Millisecond
	defer func() { chatWait = saved }()
	app := New(testDB(t), Config{Origin: origin, DevUserID: player})
	c := &client{t: t, handler: app.Handler()}
	c.signInAs(player)
	var u chatUpdate
	start := time.Now()
	c.json("GET", "/api/chats/updates?after=0", "", 200, &u)
	if time.Since(start) < chatWait || len(u.Messages) != 0 || u.Changed {
		t.Fatalf("gave up with %+v after %v", u, time.Since(start))
	}
	c.json("GET", "/api/chats/updates?after=x", "", 400, nil)
}
