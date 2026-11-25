-- The rules that give an item by themselves belong to the rewards (internal/rewards/rules): the
-- table is net.reward_rules, and each rule says in item which kind of item it gives.
--
--   item      'title' (title_id) or 'don_item' (a part of the 3D Don: don_part and don_item)
--
-- A rule is made once: two rules with the same item and the same conditions are one rule
-- (reward_rules_once). Duplicates made before are dropped here, the oldest kept; what they gave
-- stays given.
--
-- Where a player was seen is kept on their row: seen_on_site_at, signed in on the website;
-- seen_in_game_at, signed in in the game (signin.RecordActivity). The sign-in rules are judged
-- by them for everyone, as a new rule is made or the rules are gone over again. Before them
-- nothing said which: a website session marks the website, a score the game.

-- The table takes its new name, and its sequence, constraints (the primary key's index with it)
-- and indexes theirs. Applied again over itself (a database said to be behind), 0009 has just
-- made an empty net.title_rules again, which goes.
DO $$
DECLARE
  c record;
BEGIN
  IF to_regclass('net.reward_rules') IS NULL THEN
    ALTER TABLE net.title_rules RENAME TO reward_rules;
    ALTER SEQUENCE net.title_rules_id_seq RENAME TO reward_rules_id_seq;
    FOR c IN SELECT conname FROM pg_constraint
             WHERE conrelid = 'net.reward_rules'::regclass AND conname LIKE 'title\_rules%' LOOP
      EXECUTE format('ALTER TABLE net.reward_rules RENAME CONSTRAINT %I TO %I', c.conname,
        'reward_rules' || substr(c.conname, length('title_rules') + 1));
    END LOOP;
    ALTER INDEX net.title_rules_title RENAME TO reward_rules_title;
    ALTER INDEX net.title_rules_kind RENAME TO reward_rules_kind;
  ELSIF to_regclass('net.title_rules') IS NOT NULL THEN
    IF EXISTS (SELECT 1 FROM net.title_rules) THEN
      RAISE EXCEPTION 'both net.title_rules and net.reward_rules hold rules';
    END IF;
    DROP TABLE net.title_rules;
  END IF;
END $$;

ALTER TABLE net.reward_rules ADD COLUMN IF NOT EXISTS item text;
UPDATE net.reward_rules SET item = CASE WHEN title_id IS NOT NULL THEN 'title' ELSE 'don_item' END
WHERE item IS NULL;
ALTER TABLE net.reward_rules
    ALTER COLUMN item SET NOT NULL,
    DROP CONSTRAINT IF EXISTS reward_rules_item,
    ADD CONSTRAINT reward_rules_item CHECK (CASE item
        WHEN 'title' THEN title_id IS NOT NULL AND don_part IS NULL AND don_item IS NULL
        WHEN 'don_item' THEN title_id IS NULL AND don_part IS NOT NULL AND don_item IS NOT NULL
        ELSE false
    END);

-- Players upload to the fanmade source alone (the official source's charts are the built-in
-- account's), so an upload rule names no source: it counts the fanmade charts a player is an
-- owner of. One that named the official source could never be met, and goes.
DELETE FROM net.reward_rules WHERE kind = 'charts_uploaded' AND source = 'official';
ALTER TABLE net.reward_rules DROP CONSTRAINT IF EXISTS reward_rules_shape;
UPDATE net.reward_rules SET source = NULL WHERE kind = 'charts_uploaded';
ALTER TABLE net.reward_rules ADD CONSTRAINT reward_rules_shape CHECK (CASE kind
    WHEN 'chart_clear' THEN source IS NOT NULL AND chart_id IS NOT NULL AND category_flag IS NULL AND clear IS NOT NULL AND count IS NULL
    WHEN 'category_clear' THEN source IS NOT NULL AND chart_id IS NULL AND category_flag IS NOT NULL AND clear IS NOT NULL
    WHEN 'charts_uploaded' THEN source IS NULL AND chart_id IS NULL AND category_flag IS NULL AND course IS NULL AND clear IS NULL AND count IS NOT NULL
    ELSE source IS NULL AND chart_id IS NULL AND category_flag IS NULL AND course IS NULL AND clear IS NULL AND count IS NULL
END);

DELETE FROM net.reward_rules r USING net.reward_rules k
WHERE k.id < r.id
  AND (k.item, k.title_id, k.don_part, k.don_item, k.kind, k.source, k.chart_id, k.category_flag, k.course, k.clear, k.count)
      IS NOT DISTINCT FROM
      (r.item, r.title_id, r.don_part, r.don_item, r.kind, r.source, r.chart_id, r.category_flag, r.course, r.clear, r.count);
CREATE UNIQUE INDEX IF NOT EXISTS reward_rules_once ON net.reward_rules
    (item, title_id, don_part, don_item, kind, source, chart_id, category_flag, course, clear, count) NULLS NOT DISTINCT;

COMMENT ON TABLE net.reward_rules IS 'Conditions that give an item (item: a title, or a part of the 3D Don) by themselves; internal/rewards/rules. What a rule gave is an award of the service ''rule'' with request_key ''<rule id>:<user id>'', written only when the player did not hold the item; a rule gives a player its item once, so an item an administrator took back is not given again by the same rule.';
COMMENT ON COLUMN net.reward_rules.item IS 'Which kind of item the rule gives: title (title_id) or don_item (don_part, don_item).';

-- The release before this one read net.title_rules. It never runs on this database: the new
-- container replaces the old one before it migrates (deploy/publish.sh), and a release refuses
-- a database newer than itself, so there is no going back past this without a restore.

ALTER TABLE net.users
    ADD COLUMN IF NOT EXISTS seen_on_site_at timestamptz,
    ADD COLUMN IF NOT EXISTS seen_in_game_at timestamptz;
UPDATE net.users u SET seen_on_site_at = COALESCE(u.last_active_at, now())
WHERE u.seen_on_site_at IS NULL AND (EXISTS(SELECT 1 FROM net.sessions s WHERE s.user_id = u.id)
  OR EXISTS(SELECT 1 FROM net.awards a WHERE a.service = 'rule' AND a.user_id = u.id AND a.granted ->> 'kind' = 'site_login'));
UPDATE net.users u SET seen_in_game_at = COALESCE(u.last_active_at, now())
WHERE u.seen_in_game_at IS NULL AND (EXISTS(SELECT 1 FROM net.scores s WHERE s.user_id = u.id)
  OR EXISTS(SELECT 1 FROM net.awards a WHERE a.service = 'rule' AND a.user_id = u.id AND a.granted ->> 'kind' = 'game_login'));
COMMENT ON COLUMN net.users.seen_on_site_at IS 'When the player was last seen signed in on the website (signin.RecordActivity; at most once a minute). What the site_login rules go by.';
COMMENT ON COLUMN net.users.seen_in_game_at IS 'When the player was last seen signed in in the game (signin.RecordActivity; at most once a minute). What the game_login rules go by.';
