-- Rewards (internal/rewards): what a player was given and has not been shown yet. A client
-- learns how many there are from a header on any answer, fetches them and marks them seen.
--
-- The rewards package knows nothing of what a reward is: kind names it and data is what the
-- client shows, taken when it was given. What makes a reward is the triggers below, one per
-- thing that can be given; a new kind of reward is a new trigger, and the code that gives the
-- thing does not change.
CREATE TABLE IF NOT EXISTS net.rewards (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind ~ '^[a-z][a-z_]{0,31}$'),
    data jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(data) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    seen_at timestamptz
);
-- What every signed-in answer counts.
CREATE INDEX IF NOT EXISTS rewards_pending ON net.rewards (user_id, id) WHERE seen_at IS NULL;

COMMENT ON TABLE net.rewards IS 'What a player was given (kind, and data for showing it, as it was then), and when they saw it. Written by triggers on what can be given; read and marked seen by internal/rewards.';

-- A title: whoever gave it (a 段位 course, a rule, an administrator), the player now holds it.
CREATE OR REPLACE FUNCTION net.reward_title() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  INSERT INTO rewards(user_id, kind, data)
  SELECT NEW.user_id, 'title', jsonb_build_object(
    'title', jsonb_build_object('id', t.id, 'name', t.name, 'nameTranslations', t.name_translations,
      'description', t.description, 'style', t.style, 'official', t.official, 'ownerId', t.owner_id),
    'service', a.service, 'courseTitle', a.course_title)
  FROM titles t LEFT JOIN awards a ON a.id = NEW.award_id
  WHERE t.id = NEW.title_id;
  RETURN NULL;
END $$;

-- A title taken back before it was seen is not shown.
CREATE OR REPLACE FUNCTION net.unreward_title() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  DELETE FROM rewards WHERE user_id = OLD.user_id AND kind = 'title' AND seen_at IS NULL
    AND data #>> '{title,id}' = OLD.title_id;
  RETURN NULL;
END $$;

-- A 段位 badge, new or better than before. gold and combo are how the emblem is drawn (as
-- DanBadge.tiers in internal/profile/httpapi).
CREATE OR REPLACE FUNCTION net.reward_dan() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.verdict <= OLD.verdict THEN
    RETURN NULL;
  END IF;
  INSERT INTO rewards(user_id, kind, data)
  VALUES (NEW.user_id, 'dan', jsonb_build_object('index', NEW.dan, 'verdict', NEW.verdict,
    'gold', NEW.verdict >= 4, 'combo', (NEW.verdict - 1) % 3));
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS user_titles_reward ON net.user_titles;
CREATE TRIGGER user_titles_reward AFTER INSERT ON net.user_titles FOR EACH ROW EXECUTE FUNCTION net.reward_title();
DROP TRIGGER IF EXISTS user_titles_unreward ON net.user_titles;
CREATE TRIGGER user_titles_unreward AFTER DELETE ON net.user_titles FOR EACH ROW EXECUTE FUNCTION net.unreward_title();
DROP TRIGGER IF EXISTS user_dans_reward ON net.user_dans;
CREATE TRIGGER user_dans_reward AFTER INSERT OR UPDATE OF verdict ON net.user_dans FOR EACH ROW EXECUTE FUNCTION net.reward_dan();
