-- A 段位 course can be fixed, and the titles a course gives are given by rule.
--
-- A course's maker fixes it (dan_courses.fixed_at): from then on its songs and its conditions
-- change only by a request (dan_changes) that a manager of the source approves, or returns with
-- a reply. Its name and colour stay the maker's to change. An approved request is the managers'
-- own change: the course stays fixed from when it was fixed. A fixed course is what a rule may
-- name: what a clear of it means cannot change behind the rule's back.
--
-- The titles a course gave on a pass (rewards.titles, given only by official courses) become
-- rules of the kind dan_clear (internal/rewards/rules): a player whose attempt at a fixed course,
-- made since it was fixed, reached a verdict (1–3 red, 4–6 gold, with full combo or all good
-- above each) gets the rule's item. Rules belong to the title managers, and any fixed course,
-- official or not, may be named by one. A course's rewards keep only its 段位 (rewards.dan).

DO $$
DECLARE
  src text;
BEGIN
  FOREACH src IN ARRAY ARRAY['fanmade', 'official'] LOOP
    EXECUTE format($sql$
      ALTER TABLE %1$I.dan_courses ADD COLUMN IF NOT EXISTS fixed_at timestamptz;
      COMMENT ON COLUMN %1$I.dan_courses.fixed_at IS 'When the course was fixed, by its maker or a manager. NULL: not fixed, its maker changes it freely. Once fixed its songs and exams change only by an approved dan_changes request, which leaves fixed_at as it is; a dan_clear rule counts the attempts made since.';

      CREATE TABLE IF NOT EXISTS %1$I.dan_changes (
          id text PRIMARY KEY CONSTRAINT dan_changes_id CHECK (id ~ '^[0-9a-f]{32}$'),
          course_id text NOT NULL REFERENCES %1$I.dan_courses (id) ON DELETE CASCADE,
          user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
          songs jsonb NOT NULL CONSTRAINT dan_changes_songs CHECK (jsonb_typeof(songs) = 'array'),
          exams jsonb NOT NULL CONSTRAINT dan_changes_exams CHECK (jsonb_typeof(exams) = 'array'),
          request text NOT NULL CONSTRAINT dan_changes_request CHECK (char_length(request) BETWEEN 1 AND 500),
          status text NOT NULL DEFAULT 'pending'
              CONSTRAINT dan_changes_status CHECK (status IN ('pending', 'approved', 'returned', 'withdrawn')),
          reviewer_id text REFERENCES net.users (id) ON DELETE SET NULL,
          reply text NOT NULL DEFAULT '' CONSTRAINT dan_changes_reply CHECK (char_length(reply) <= 500),
          created_at timestamptz NOT NULL DEFAULT now(),
          decided_at timestamptz,
          CONSTRAINT dan_changes_decided CHECK ((status = 'pending') = (decided_at IS NULL))
      );
      CREATE UNIQUE INDEX IF NOT EXISTS dan_changes_pending ON %1$I.dan_changes (course_id) WHERE status = 'pending';
      CREATE INDEX IF NOT EXISTS dan_changes_queue ON %1$I.dan_changes (status, created_at, id);
      CREATE INDEX IF NOT EXISTS dan_changes_course ON %1$I.dan_changes (course_id, created_at DESC);
      COMMENT ON TABLE %1$I.dan_changes IS 'A request to change the songs and exams of a fixed 段位 course, as they are asked to be (songs: [{chartId, course, side, hidden}]). A manager of the source approves it, which copies them onto the course (it stays fixed from when it was), or returns it with a reply. One pending request per course.';
    $sql$, src);
  END LOOP;
END $$;

-- The rules: a dan_clear rule names a source and a course of it (dan_id), and the verdict it
-- takes (clear, 1–6). The other kinds' clear stays a chart's clear status (1–3).
ALTER TABLE net.reward_rules ADD COLUMN IF NOT EXISTS dan_id text;
ALTER TABLE net.reward_rules
    DROP CONSTRAINT IF EXISTS reward_rules_kind_check,
    ADD CONSTRAINT reward_rules_kind_check CHECK (kind IN ('chart_clear', 'category_clear', 'dan_clear', 'site_login', 'game_login', 'charts_uploaded')),
    DROP CONSTRAINT IF EXISTS reward_rules_clear_check,
    ADD CONSTRAINT reward_rules_clear_check CHECK (clear BETWEEN 1 AND 6),
    DROP CONSTRAINT IF EXISTS reward_rules_shape,
    ADD CONSTRAINT reward_rules_shape CHECK (CASE kind
        WHEN 'chart_clear' THEN source IS NOT NULL AND chart_id IS NOT NULL AND dan_id IS NULL AND category_flag IS NULL AND clear IS NOT NULL AND clear <= 3 AND count IS NULL
        WHEN 'category_clear' THEN source IS NOT NULL AND chart_id IS NULL AND dan_id IS NULL AND category_flag IS NOT NULL AND clear IS NOT NULL AND clear <= 3
        WHEN 'dan_clear' THEN source IS NOT NULL AND chart_id IS NULL AND dan_id IS NOT NULL AND category_flag IS NULL AND course IS NULL AND clear IS NOT NULL AND count IS NULL
        WHEN 'charts_uploaded' THEN source IS NULL AND chart_id IS NULL AND dan_id IS NULL AND category_flag IS NULL AND course IS NULL AND clear IS NULL AND count IS NOT NULL
        ELSE source IS NULL AND chart_id IS NULL AND dan_id IS NULL AND category_flag IS NULL AND course IS NULL AND clear IS NULL AND count IS NULL
    END);
DROP INDEX IF EXISTS net.reward_rules_once;
CREATE UNIQUE INDEX reward_rules_once ON net.reward_rules
    (item, title_id, don_part, don_item, kind, source, chart_id, dan_id, category_flag, course, clear, count) NULLS NOT DISTINCT;
CREATE INDEX IF NOT EXISTS reward_rules_dan ON net.reward_rules (source, dan_id) WHERE dan_id IS NOT NULL;
COMMENT ON COLUMN net.reward_rules.dan_id IS 'The 段位 course of a dan_clear rule, in its source. Only a fixed course is met (dan_courses.fixed_at), by the attempts made since it was fixed; clear is then the verdict, 1–6.';

-- What official courses gave becomes rules, and they are fixed from their last change, so that
-- the attempts that earned their titles count. A course that was not official never gave its
-- titles: what its maker named is dropped. Titles a player already holds stay held; a rule gives
-- nobody what they hold.
DO $$
DECLARE
  src text;
BEGIN
  FOREACH src IN ARRAY ARRAY['fanmade', 'official'] LOOP
    EXECUTE format($sql$
      INSERT INTO net.reward_rules (item, title_id, kind, source, dan_id, clear)
      SELECT DISTINCT 'title', t.id, 'dan_clear', %1$L, d.id,
             CASE r.reward ->> 'on' WHEN 'red' THEN 1 WHEN 'red1' THEN 1 WHEN 'red2' THEN 2 WHEN 'red3' THEN 3
                                    WHEN 'gold' THEN 4 WHEN 'gold4' THEN 4 WHEN 'gold5' THEN 5 WHEN 'gold6' THEN 6 END
      FROM %1$I.dan_courses d
      CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(d.rewards -> 'titles') = 'array' THEN d.rewards -> 'titles' ELSE '[]' END) r(reward)
      JOIN net.titles t ON t.id = r.reward ->> 'id' AND t.visibility = 'public'
      WHERE d.official AND d.status = 'published' AND r.reward ->> 'on' IN ('red', 'red1', 'red2', 'red3', 'gold', 'gold4', 'gold5', 'gold6')
      ON CONFLICT DO NOTHING;

      UPDATE %1$I.dan_courses d SET fixed_at = d.updated_at
      WHERE d.fixed_at IS NULL AND EXISTS (SELECT 1 FROM net.reward_rules r WHERE r.source = %1$L AND r.dan_id = d.id);

      UPDATE %1$I.dan_courses SET rewards = rewards - 'titles' WHERE rewards ? 'titles';
    $sql$, src);
  END LOOP;
END $$;

COMMENT ON TABLE net.reward_rules IS 'Conditions that give an item (item: a title, or a part of the 3D Don) by themselves; internal/rewards/rules. What a rule gave is an award of the service ''rule'' with request_key ''<rule id>:<user id>'', written only when the player did not hold the item; a rule gives a player its item once, so an item an administrator took back is not given again by the same rule.';
