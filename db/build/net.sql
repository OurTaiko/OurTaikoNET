-- What ourtaiko_net holds once for the whole site, added after both sources were adopted
-- (adopt.sql already moved users, sessions and oidc_flows here).
\set ON_ERROR_STOP 1
SET search_path = net;

COMMENT ON SCHEMA net IS 'Shared by both song sources: players, sign-in, roles, titles, and one parent table over each source''s scores and dan attempts.';

CREATE TABLE schema_migrations (
  version int PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

-- Who may do what. The site administrator is the SSO Application's administrator and is not
-- listed here; that person grants these. source NULL means both sources.
CREATE TABLE roles (
  user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('admin')),
  source text CHECK (source IN ('fanmade', 'official')),
  granted_by text REFERENCES users (id) ON DELETE SET NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (user_id, role, source)
);

-- Every play of both sources. fanmade.scores and official.scores stay the tables they were,
-- each with its foreign key to its own chart_difficulties; this is the two together.
-- A row is named by (source, id): each source keeps its own primary key on id.
CREATE TABLE scores (LIKE fanmade.scores INCLUDING DEFAULTS) PARTITION BY LIST (source);
ALTER TABLE scores ALTER COLUMN source DROP DEFAULT;
ALTER TABLE scores ATTACH PARTITION fanmade.scores FOR VALUES IN ('fanmade');
ALTER TABLE scores ATTACH PARTITION official.scores FOR VALUES IN ('official');
ALTER TABLE scores ADD CONSTRAINT scores_source_id_key UNIQUE (source, id);
CREATE INDEX scores_player_recent ON scores (user_id, submitted_at DESC, id DESC);

CREATE TABLE dan_attempts (LIKE fanmade.dan_attempts INCLUDING DEFAULTS) PARTITION BY LIST (source);
ALTER TABLE dan_attempts ALTER COLUMN source DROP DEFAULT;
ALTER TABLE dan_attempts ATTACH PARTITION fanmade.dan_attempts FOR VALUES IN ('fanmade');
ALTER TABLE dan_attempts ATTACH PARTITION official.dan_attempts FOR VALUES IN ('official');
ALTER TABLE dan_attempts ADD CONSTRAINT dan_attempts_source_id_key UNIQUE (source, id);
CREATE INDEX dan_attempts_player_recent ON dan_attempts (user_id, submitted_at DESC, id DESC);

-- From OurTaikoProfile (its schema 2), with players now rows of users.
-- OurTaikoProfile had no table of players: whoever it was told about simply appeared in its
-- rows. Here those rows point at users, so a player first heard of this way gets a users row.
CREATE FUNCTION ensure_player() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
DECLARE
  player text := to_jsonb(NEW) ->> TG_ARGV[0];
BEGIN
  -- Looked up first: nearly always the player is there already and nothing is written.
  IF player IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users WHERE id = player) THEN
    INSERT INTO users (id) VALUES (player) ON CONFLICT (id) DO NOTHING;
  END IF;
  RETURN NEW;
END $$;

-- Who may ask for awards: each song source, one key each, kept as a hash. Both sources now
-- run in this program, which gives them fresh keys every time it starts.
CREATE TABLE services (
  id text PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{0,39}$'),
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  key_hash text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Titles. style names the game's built-in title band. owner_id is the player who made a
-- community title; the ones administrators keep have none.
CREATE TABLE titles (
  id text PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
  name_translations jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(name_translations) = 'object'),
  description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
  style text NOT NULL CHECK (style IN ('wood', 'teal', 'wood2', 'purple', 'gold', 'rainbow')),
  official boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  owner_id text REFERENCES users (id)
);
CREATE INDEX titles_owner ON titles (owner_id) WHERE owner_id IS NOT NULL;

-- Every award asked for, once per (service, request_key): a dan attempt of a source
-- ('fanmade', 'official'; request_key is the attempt's id) or a grant by an administrator
-- ('admin'). granted records what it actually gave.
CREATE TABLE awards (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  service text NOT NULL,
  request_key text NOT NULL CHECK (length(request_key) BETWEEN 1 AND 100),
  user_id text NOT NULL REFERENCES users (id),
  course_id text NOT NULL DEFAULT '',
  course_title text NOT NULL DEFAULT '',
  official boolean NOT NULL DEFAULT false,
  verdict smallint NOT NULL DEFAULT 0 CHECK (verdict BETWEEN 0 AND 6),
  dan smallint CHECK (dan BETWEEN 0 AND 24),
  granted jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (service, request_key)
);
CREATE INDEX awards_user ON awards (user_id, created_at DESC);

CREATE TABLE user_titles (
  user_id text NOT NULL REFERENCES users (id),
  title_id text NOT NULL REFERENCES titles (id) ON DELETE CASCADE,
  award_id bigint REFERENCES awards (id) ON DELETE SET NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, title_id)
);

-- The best result of each dan rank from official courses; it only ever goes up.
CREATE TABLE user_dans (
  user_id text NOT NULL REFERENCES users (id),
  dan smallint NOT NULL CHECK (dan BETWEEN 0 AND 24),
  verdict smallint NOT NULL CHECK (verdict BETWEEN 1 AND 6),
  award_id bigint REFERENCES awards (id) ON DELETE SET NULL,
  achieved_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, dan)
);

-- What a player wears: one of their titles (or none) and whether the highest dan shows.
CREATE TABLE nameplates (
  user_id text PRIMARY KEY REFERENCES users (id),
  title_id text REFERENCES titles (id) ON DELETE SET NULL,
  show_dan boolean NOT NULL DEFAULT true,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER titles_ensure_player BEFORE INSERT OR UPDATE OF owner_id ON titles FOR EACH ROW EXECUTE FUNCTION ensure_player('owner_id');
CREATE TRIGGER awards_ensure_player BEFORE INSERT OR UPDATE OF user_id ON awards FOR EACH ROW EXECUTE FUNCTION ensure_player('user_id');
CREATE TRIGGER user_titles_ensure_player BEFORE INSERT OR UPDATE OF user_id ON user_titles FOR EACH ROW EXECUTE FUNCTION ensure_player('user_id');
CREATE TRIGGER user_dans_ensure_player BEFORE INSERT OR UPDATE OF user_id ON user_dans FOR EACH ROW EXECUTE FUNCTION ensure_player('user_id');
CREATE TRIGGER nameplates_ensure_player BEFORE INSERT OR UPDATE OF user_id ON nameplates FOR EACH ROW EXECUTE FUNCTION ensure_player('user_id');
-- Signing in at the players' pages is also a first appearance: the song sources record a
-- player before opening a session, those pages do not.
CREATE TRIGGER sessions_ensure_player BEFORE INSERT OR UPDATE OF user_id ON sessions FOR EACH ROW EXECUTE FUNCTION ensure_player('user_id');

INSERT INTO schema_migrations (version) VALUES (1);
