-- Comments under the site's announcements (internal/profile/httpapi/announcement_comments.go).
-- They are package comments' threads, as under a song or a 段位 course of a song source, so
-- the tables have the names and the shape that package reads: comments and comment_votes, here
-- in schema net, with the announcement as the one thing a comment hangs on. A song source's
-- connection finds its own tables of these names first (its search_path is the source, then
-- net); the site's own connection finds these.
ALTER TABLE net.announcements ADD COLUMN IF NOT EXISTS comment_count integer NOT NULL DEFAULT 0 CHECK (comment_count >= 0);

CREATE TABLE IF NOT EXISTS net.comments (
    id text PRIMARY KEY,
    announcement_id text NOT NULL REFERENCES net.announcements(id) ON DELETE CASCADE,
    parent_id text REFERENCES net.comments(id) ON DELETE CASCADE,
    -- The top-level comment of its thread: itself when it is one.
    root_id text NOT NULL,
    depth integer NOT NULL CHECK (depth >= 0 AND depth <= 1000),
    author_id text NOT NULL REFERENCES net.users(id),
    body text NOT NULL CHECK (char_length(body) <= 10000),
    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at timestamptz,
    -- Deleted by its author, or removed by an administrator: the row keeps its place for the
    -- replies under it, without its text.
    deleted_at timestamptz,
    removed_at timestamptz,
    upvotes integer NOT NULL DEFAULT 0 CHECK (upvotes >= 0),
    downvotes integer NOT NULL DEFAULT 0 CHECK (downvotes >= 0),
    reply_count integer NOT NULL DEFAULT 0 CHECK (reply_count >= 0),
    CHECK ((parent_id IS NULL) = (depth = 0) AND (parent_id IS NOT NULL OR root_id = id)),
    CHECK ((deleted_at IS NULL AND removed_at IS NULL) = (body <> ''))
);

COMMENT ON TABLE net.comments IS 'Threaded comments under the site''s announcements, in the shape package comments reads; a song source keeps its own table of this name.';

CREATE INDEX IF NOT EXISTS comments_announcement_roots ON net.comments (announcement_id, created_at DESC) WHERE parent_id IS NULL;
CREATE INDEX IF NOT EXISTS comments_author ON net.comments (author_id, created_at DESC);
CREATE INDEX IF NOT EXISTS comments_parent ON net.comments (parent_id);
CREATE INDEX IF NOT EXISTS comments_root ON net.comments (root_id);

-- One upvote (1) or 何意味 (-1) per player per comment.
CREATE TABLE IF NOT EXISTS net.comment_votes (
    comment_id text NOT NULL REFERENCES net.comments(id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES net.users(id),
    value smallint NOT NULL CHECK (value IN (-1, 1)),
    voted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (comment_id, user_id)
);

COMMENT ON TABLE net.comment_votes IS 'Players'' votes on the comments under announcements: 1 an upvote, -1 a 何意味.';

CREATE INDEX IF NOT EXISTS comment_votes_user ON net.comment_votes (user_id);

-- The two orders that weigh votes, as each song source has them: the lower bound of the
-- share of upvotes (best), and how evenly a comment divides its voters (controversial).
CREATE OR REPLACE FUNCTION net.comment_confidence(up integer, down integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = net AS $$
 SELECT CASE WHEN up+down=0 THEN 0 ELSE
  (up::float8/(up+down) + 1.642374415149/(2*(up+down))
   - 1.281551565545*sqrt((up::float8/(up+down)*(1-up::float8/(up+down)) + 1.642374415149/(4*(up+down)))/(up+down)))
  / (1 + 1.642374415149/(up+down)) END
$$;

CREATE OR REPLACE FUNCTION net.comment_controversy(up integer, down integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = net AS $$
 SELECT CASE WHEN up<=0 OR down<=0 THEN 0
  ELSE power((up+down)::float8, CASE WHEN up>down THEN down::float8/up ELSE up::float8/down END) END
$$;

-- A comment counts in its parent's replies and in its announcement's comments.
CREATE OR REPLACE FUNCTION net.count_comment() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
DECLARE delta integer; rec comments;
BEGIN
 IF TG_OP='INSERT' THEN delta:=1; rec:=NEW; ELSE delta:=-1; rec:=OLD; END IF;
 IF rec.parent_id IS NOT NULL THEN
  UPDATE comments SET reply_count=reply_count+delta WHERE id=rec.parent_id;
 END IF;
 UPDATE announcements SET comment_count=comment_count+delta WHERE id=rec.announcement_id;
 RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS comments_count ON net.comments;
CREATE TRIGGER comments_count AFTER INSERT OR DELETE ON net.comments FOR EACH ROW EXECUTE FUNCTION net.count_comment();

-- A vote counts in its comment's tallies.
CREATE OR REPLACE FUNCTION net.count_comment_vote() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
DECLARE target text; up integer:=0; down integer:=0;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  target:=OLD.comment_id;
  IF OLD.value=1 THEN up:=up-1; ELSE down:=down-1; END IF;
 END IF;
 IF TG_OP IN ('INSERT','UPDATE') THEN
  target:=NEW.comment_id;
  IF NEW.value=1 THEN up:=up+1; ELSE down:=down+1; END IF;
 END IF;
 UPDATE comments SET upvotes=upvotes+up,downvotes=downvotes+down WHERE id=target;
 RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS comment_votes_count ON net.comment_votes;
CREATE TRIGGER comment_votes_count AFTER INSERT OR DELETE OR UPDATE OF value ON net.comment_votes FOR EACH ROW EXECUTE FUNCTION net.count_comment_vote();
