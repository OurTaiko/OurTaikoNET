-- Titles are private or public, where they were community or official.
--
-- A player makes a title and it is private: theirs alone, held by them from the start, changed
-- or deleted as they like, and one of the five such titles a player may keep. Nobody else can be
-- given it. To have it given to others the maker asks for a review (net.title_reviews), saying
-- how it should be obtained; a title manager approves or returns the request, with a reply. An
-- approved title is public: it stands in the catalog with how it is obtained (obtain, and the
-- rules of net.title_rules), frees its maker's place, and from then on only title managers decide
-- who gets it (rules, official 段位 courses, grants by hand). A public title changes only by a
-- review of the change its maker asks for, or by a title manager.
--
-- The titles the site's managers make themselves have no owner and are public from the start.

ALTER TABLE net.titles
    ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'private'
        CONSTRAINT titles_visibility CHECK (visibility IN ('private', 'public')),
    ADD COLUMN IF NOT EXISTS obtain text NOT NULL DEFAULT ''
        CONSTRAINT titles_obtain CHECK (char_length(obtain) <= 500);

-- What was official, or kept by the managers without a maker, is public; a title a player made
-- and nobody made official is private. official goes: visibility says it.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'net' AND table_name = 'titles' AND column_name = 'official') THEN
    UPDATE net.titles SET visibility = CASE WHEN official OR owner_id IS NULL THEN 'public' ELSE 'private' END;
    ALTER TABLE net.titles DROP COLUMN official;
  END IF;
END $$;

-- Whoever makes a title says which it is: there is no default.
ALTER TABLE net.titles
    ALTER COLUMN visibility DROP DEFAULT,
    DROP CONSTRAINT IF EXISTS titles_private_owned,
    ADD CONSTRAINT titles_private_owned CHECK (visibility = 'public' OR owner_id IS NOT NULL);

COMMENT ON COLUMN net.titles.visibility IS 'private: its maker''s alone (owner_id), one of the five a player may keep, given to nobody else. public: in the catalog, given by what title managers set. A title becomes public by an approved review, or is made public by a manager (no owner).';
COMMENT ON COLUMN net.titles.obtain IS 'How the title is obtained, in words, shown in the catalog beside its rules. Written by the title manager who approved it (from what its maker asked for) or made it.';

-- A title given is shown as it looks. Its maker holds it from the start, which is no reward.
-- official is what clients from before visibility read: a public title.
CREATE OR REPLACE FUNCTION net.reward_title() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  INSERT INTO rewards(user_id, kind, data)
  SELECT NEW.user_id, 'title', jsonb_build_object(
    'title', jsonb_build_object('id', t.id, 'name', t.name, 'nameTranslations', t.name_translations,
      'description', t.description, 'obtain', t.obtain, 'style', t.style,
      'visibility', t.visibility, 'official', t.visibility = 'public', 'ownerId', t.owner_id,
      'layers', jsonb_build_object('band', t.band_item, 'effect', t.effect_item, 'icon', t.icon_item)),
    'service', a.service, 'courseTitle', a.course_title)
  FROM titles t LEFT JOIN awards a ON a.id = NEW.award_id
  WHERE t.id = NEW.title_id AND t.owner_id IS DISTINCT FROM NEW.user_id;
  RETURN NULL;
END $$;

-- Every maker holds the titles they made before.
INSERT INTO net.user_titles (user_id, title_id)
SELECT owner_id, id FROM net.titles WHERE owner_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- A request to make a private title public (publish), or to change a public one (change), with
-- the title as it is asked to be: an approved request's name, description and look become the
-- title's. While one is pending its title cannot be changed.
CREATE TABLE IF NOT EXISTS net.title_reviews (
    id text PRIMARY KEY CONSTRAINT title_reviews_id CHECK (id ~ '^[0-9a-f]{32}$'),
    title_id text NOT NULL REFERENCES net.titles (id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
    kind text NOT NULL CONSTRAINT title_reviews_kind CHECK (kind IN ('publish', 'change')),
    name text NOT NULL CONSTRAINT title_reviews_name CHECK (char_length(name) BETWEEN 1 AND 40),
    name_translations jsonb NOT NULL DEFAULT '{}'
        CONSTRAINT title_reviews_name_translations CHECK (jsonb_typeof(name_translations) = 'object'),
    description text NOT NULL DEFAULT '' CONSTRAINT title_reviews_description CHECK (char_length(description) <= 500),
    band_item text NOT NULL CONSTRAINT title_reviews_band_item CHECK (band_item ~ '^band/[a-z0-9][a-z0-9_.-]{0,63}$'),
    effect_item text CONSTRAINT title_reviews_effect_item CHECK (effect_item ~ '^effect/[a-z0-9][a-z0-9_.-]{0,63}$'),
    icon_item text CONSTRAINT title_reviews_icon_item CHECK (icon_item ~ '^icon/[a-z0-9][a-z0-9_.-]{0,63}$'),
    request text NOT NULL CONSTRAINT title_reviews_request CHECK (char_length(request) BETWEEN 1 AND 500),
    status text NOT NULL DEFAULT 'pending'
        CONSTRAINT title_reviews_status CHECK (status IN ('pending', 'approved', 'returned', 'withdrawn')),
    reviewer_id text REFERENCES net.users (id) ON DELETE SET NULL,
    reply text NOT NULL DEFAULT '' CONSTRAINT title_reviews_reply CHECK (char_length(reply) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    CONSTRAINT title_reviews_decided CHECK ((status = 'pending') = (decided_at IS NULL))
);
-- One request at a time for a title.
CREATE UNIQUE INDEX IF NOT EXISTS title_reviews_pending ON net.title_reviews (title_id) WHERE status = 'pending';
-- The managers' queue (pending, oldest first) and what was decided (newest first).
CREATE INDEX IF NOT EXISTS title_reviews_queue ON net.title_reviews (status, created_at, id);
-- A title's latest request.
CREATE INDEX IF NOT EXISTS title_reviews_title ON net.title_reviews (title_id, created_at DESC);

COMMENT ON TABLE net.title_reviews IS 'A maker''s request that title managers review their title: publish makes a private title public, change changes a public one. Holds the title as asked for; approving copies it onto the title. One pending request per title.';
COMMENT ON COLUMN net.title_reviews.request IS 'What the maker tells the title managers: how the title should be obtained, or what the change is for.';
COMMENT ON COLUMN net.title_reviews.reply IS 'What the title manager who decided, or nobody for a withdrawn request, answered.';

-- 称号中心: what a player is told about titles (a request reviewed, a request to review, a title
-- of theirs removed), read like a chat among the player's chats. data is what the website shows,
-- as it was then: a title named in it may have changed or gone since.
CREATE TABLE IF NOT EXISTS net.title_notices (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
    kind text NOT NULL CONSTRAINT title_notices_kind CHECK (kind ~ '^[a-z][a-z_]{0,31}$'),
    data jsonb NOT NULL DEFAULT '{}' CONSTRAINT title_notices_data CHECK (jsonb_typeof(data) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz
);
CREATE INDEX IF NOT EXISTS title_notices_list ON net.title_notices (user_id, id DESC);
CREATE INDEX IF NOT EXISTS title_notices_unread ON net.title_notices (user_id) WHERE read_at IS NULL;

COMMENT ON TABLE net.title_notices IS '称号中心: notices to a player about titles (kind, and data for showing it as it was then), and when they read it. Written by internal/profile/httpapi where a review is asked for or decided and where a manager removes a player''s title.';
