-- Fanmade's categories grow from six to fourteen, and what players suggest about a song is a
-- category now, not a tag. The tags of 0026_chart_tags lasted a day: the charts that have one
-- are given the category it became, and the tag tables are left unread (the release before
-- this one reads them while it is replaced) for a later migration to drop.
--
-- The new categories, one bit each after the six that were (internal/songs/httpapi/sources.go):
-- touhou 64, gekichumai 128, bemani 256, project-sekai 512, other-rhythm-games 1024, bms 2048,
-- album 4096, internet 8192. The tags anime, pops, vsingers and game-original became the
-- categories anime 32, pop 4, virtual-singer 2 and game 1 that were there already. The official
-- source keeps the eight categories of the official game.

ALTER TABLE fanmade.charts DROP CONSTRAINT IF EXISTS charts_category_flags_check,
    ADD CONSTRAINT charts_category_flags_check CHECK (category_flags >= 0 AND (category_flags & 16383) = category_flags);
ALTER TABLE fanmade.playlists DROP CONSTRAINT IF EXISTS playlists_category_flag_check,
    ADD CONSTRAINT playlists_category_flag_check CHECK (category_flag > 0 AND category_flag <= 8192 AND (category_flag & (category_flag - 1)) = 0);

-- A tag's category bit in fanmade.
CREATE TEMPORARY TABLE tag_bits (tag_id text PRIMARY KEY, bit integer NOT NULL) ON COMMIT DROP;
INSERT INTO tag_bits VALUES ('game-original', 1), ('vsingers', 2), ('pops', 4), ('anime', 32),
    ('touhou', 64), ('gekichumai', 128), ('bemani', 256), ('project-sekai', 512),
    ('other-rhythm-games', 1024), ('bms', 2048), ('album', 4096), ('internet', 8192);

-- A chart given a category by its tags leaves Variety, which is where a chart is when it has
-- nowhere else to be.
UPDATE fanmade.charts c SET category_flags = (c.category_flags | t.bits) & ~16
FROM (SELECT ct.chart_id, bit_or(b.bit) AS bits FROM fanmade.chart_tags ct JOIN tag_bits b USING (tag_id) GROUP BY ct.chart_id) t
WHERE c.id = t.chart_id AND (c.category_flags | t.bits) <> c.category_flags;

-- A player's suggestion that a fanmade chart is in a category, and the categories a manager of
-- the source decided a chart is not in. Both sources have the tables (they keep the same set);
-- only Fanmade takes suggestions.
DO $$
DECLARE
  src text;
BEGIN
  FOREACH src IN ARRAY ARRAY['fanmade', 'official'] LOOP
    EXECUTE format($sql$
      CREATE TABLE IF NOT EXISTS %1$I.category_rejections (
          chart_id text NOT NULL REFERENCES %1$I.charts (id) ON DELETE CASCADE,
          category_flag integer NOT NULL CONSTRAINT category_rejections_flag CHECK (category_flag > 0 AND (category_flag & (category_flag - 1)) = 0),
          rejected_by text REFERENCES net.users (id) ON DELETE SET NULL,
          rejected_at timestamptz NOT NULL DEFAULT now(),
          PRIMARY KEY (chart_id, category_flag)
      );
      COMMENT ON TABLE %1$I.category_rejections IS 'A category a manager of the source decided a chart is not in: later suggestions of it are filed rejected. Putting the chart in the category removes the row.';

      CREATE TABLE IF NOT EXISTS %1$I.category_suggestions (
          chart_id text NOT NULL REFERENCES %1$I.charts (id) ON DELETE CASCADE,
          category_flag integer NOT NULL CONSTRAINT category_suggestions_flag CHECK (category_flag > 0 AND (category_flag & (category_flag - 1)) = 0),
          user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
          created_at timestamptz NOT NULL DEFAULT now(),
          status text NOT NULL DEFAULT 'pending'
              CONSTRAINT category_suggestions_status CHECK (status IN ('pending', 'accepted', 'rejected')),
          -- The manager who decided; NULL for a suggestion filed after the decision.
          decided_by text REFERENCES net.users (id) ON DELETE SET NULL,
          decided_at timestamptz,
          PRIMARY KEY (chart_id, category_flag, user_id),
          CONSTRAINT category_suggestions_decided CHECK ((status = 'pending') = (decided_at IS NULL))
      );
      CREATE INDEX IF NOT EXISTS category_suggestions_queue ON %1$I.category_suggestions (status, chart_id, category_flag);
      CREATE INDEX IF NOT EXISTS category_suggestions_user ON %1$I.category_suggestions (user_id);
      COMMENT ON TABLE %1$I.category_suggestions IS 'A player''s suggestion that a chart is in a category (its bit). A manager''s decision answers every suggestion of that category for that chart, and a suggestion made after it is filed with it (accepted when the chart is in the category, rejected when category_rejections has it).';
    $sql$, src);
  END LOOP;
END $$;

-- What the tags' suggestions and rejections said, said of the categories.
INSERT INTO fanmade.category_rejections (chart_id, category_flag, rejected_by, rejected_at)
SELECT r.chart_id, b.bit, r.rejected_by, r.rejected_at FROM fanmade.tag_rejections r JOIN tag_bits b USING (tag_id)
WHERE NOT EXISTS (SELECT 1 FROM fanmade.charts c WHERE c.id = r.chart_id AND c.category_flags & b.bit <> 0)
ON CONFLICT DO NOTHING;
INSERT INTO fanmade.category_suggestions (chart_id, category_flag, user_id, created_at, status, decided_by, decided_at)
SELECT s.chart_id, b.bit, s.user_id, s.created_at,
       CASE WHEN c.category_flags & b.bit <> 0 AND s.status = 'pending' THEN 'accepted' ELSE s.status END,
       s.decided_by, CASE WHEN c.category_flags & b.bit <> 0 AND s.status = 'pending' THEN now() ELSE s.decided_at END
FROM fanmade.tag_suggestions s JOIN tag_bits b USING (tag_id) JOIN fanmade.charts c ON c.id = s.chart_id
ON CONFLICT DO NOTHING;

-- TAG_MANAGER (bit 8) is no group any more: a source's managers decide its suggestions.
UPDATE net.users SET groups = groups & ~8 WHERE groups & 8 <> 0;

COMMENT ON TABLE net.tags IS 'Unread since 0027_fanmade_categories (the tags became Fanmade categories); to be dropped with each source''s chart_tags, tag_rejections and tag_suggestions.';
