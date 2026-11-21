package httpapi

import (
	"context"
	"testing"

	"ourtaiko.dev/net/internal/comments"
)

func TestPlayersCommentOnAnAnnouncement(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	c := &client{t: t, handler: New(pool, Config{Origin: origin, DevUserID: player}).Handler()}
	c.signIn()
	var told Announcement
	c.json("POST", "/api/admin/announcements", `{"title":"Maintenance"}`, 201, &told)
	under := "/api/announcements/" + told.ID + "/comments"

	// Anyone reads the comments; only a signed-in player writes one. There are none of an
	// announcement that is none.
	guest := &client{t: t, handler: c.handler}
	var page comments.Page
	guest.json("GET", under, "", 200, &page)
	if page.Total != 0 || page.CommentCount != 0 || page.Items == nil {
		t.Fatalf("comments of a new announcement: %+v", page)
	}
	guest.json("POST", under, `{"body":"hello"}`, 401, nil)
	c.json("GET", "/api/announcements/none/comments", "", 404, nil)
	c.json("POST", "/api/announcements/none/comments", `{"body":"hello"}`, 404, nil)

	// A comment and a reply under it: both count for the announcement, the reply hangs under
	// its parent, and each starts upvoted by its author.
	var first, reply comments.Comment
	c.json("POST", under, `{"body":"  When exactly?  "}`, 201, &first)
	if first.Body != "When exactly?" || first.AnnouncementID != told.ID || first.AuthorID != player || first.Upvotes != 1 || first.MyVote != 1 || first.Depth != 0 {
		t.Fatalf("comment %+v", first)
	}
	c.json("POST", under, `{"body":"At nine.","parentId":"`+first.ID+`"}`, 201, &reply)
	if reply.ParentID == nil || *reply.ParentID != first.ID || reply.Depth != 1 {
		t.Fatalf("reply %+v", reply)
	}
	c.json("GET", under+"?sort=old", "", 200, &page)
	if page.Total != 1 || page.CommentCount != 2 || len(page.Items) != 1 || len(page.Items[0].Replies) != 1 || page.Items[0].Replies[0].ID != reply.ID || page.Items[0].ReplyCount != 1 {
		t.Fatalf("thread %+v", page)
	}
	var shown Announcement
	c.json("GET", "/api/announcements/"+told.ID, "", 200, &shown)
	if shown.CommentCount != 2 || shown.Title != "Maintenance" {
		t.Fatalf("announcement %+v", shown)
	}
	c.json("GET", "/api/announcements/none", "", 404, nil)
	// A reply belongs to its parent's announcement.
	var second Announcement
	c.json("POST", "/api/admin/announcements", `{"title":"Another"}`, 201, &second)
	c.json("POST", "/api/announcements/"+second.ID+"/comments", `{"body":"x","parentId":"`+first.ID+`"}`, 404, nil)

	// The permalink of a comment is the comment with what is under it.
	var thread struct{ Item comments.Comment }
	guest.json("GET", "/api/comments/"+first.ID, "", 200, &thread)
	if thread.Item.ID != first.ID || len(thread.Item.Replies) != 1 || thread.Item.MyVote != 0 {
		t.Fatalf("permalink %+v", thread)
	}

	// Its author edits it and changes their vote; the tallies follow.
	var edited comments.Comment
	c.json("PATCH", "/api/comments/"+first.ID, `{"body":"When exactly, in UTC?"}`, 200, &edited)
	if edited.Body != "When exactly, in UTC?" || edited.EditedAt == nil {
		t.Fatalf("edited %+v", edited)
	}
	var tally comments.VoteResult
	c.json("PUT", "/api/comments/"+first.ID+"/vote", `{"value":-1}`, 200, &tally)
	if tally.Upvotes != 0 || tally.Downvotes != 1 || tally.MyVote != -1 {
		t.Fatalf("tally %+v", tally)
	}
	c.json("PUT", "/api/comments/"+first.ID+"/vote", `{"value":2}`, 400, nil)

	// Another player's comment: the site's administrator (the development user) removes it.
	// With a reply under it, it keeps its place without its text.
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1)`, other); err != nil {
		t.Fatal(err)
	}
	theirs := "000000000000000000000000000000aa"
	if _, err := pool.Exec(ctx, `INSERT INTO net.comments(id,announcement_id,root_id,depth,author_id,body) VALUES ($1,$2,$1,0,$3,'spam')`, theirs, told.ID, other); err != nil {
		t.Fatal(err)
	}
	c.json("PATCH", "/api/comments/"+theirs, `{"body":"mine now"}`, 403, nil)
	c.json("POST", under, `{"body":"Please don't.","parentId":"`+theirs+`"}`, 201, nil)
	var gone map[string]bool
	c.json("DELETE", "/api/comments/"+theirs, "", 200, &gone)
	if gone["purged"] {
		t.Fatal("a removed comment with a reply was purged")
	}
	guest.json("GET", "/api/comments/"+theirs, "", 200, &thread)
	if !thread.Item.Removed || thread.Item.Body != "" || thread.Item.AuthorID != "" || len(thread.Item.Replies) != 1 {
		t.Fatalf("removed %+v", thread.Item)
	}

	// One without replies is gone altogether, and so is everything under an announcement
	// taken down.
	c.json("DELETE", "/api/comments/"+reply.ID, "", 200, &gone)
	if !gone["purged"] {
		t.Fatal("a comment without replies was kept")
	}
	c.json("GET", "/api/announcements/"+told.ID, "", 200, &shown)
	if shown.CommentCount != 3 {
		t.Fatalf("%d comments counted, want 3", shown.CommentCount)
	}
	c.json("DELETE", "/api/admin/announcements/"+told.ID, "", 204, nil)
	var left int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM net.comments)+(SELECT count(*) FROM net.comment_votes)`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d rows left under a deleted announcement (%v)", left, err)
	}
	guest.json("GET", "/api/comments/"+first.ID, "", 404, nil)
}
