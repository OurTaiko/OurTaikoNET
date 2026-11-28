-- Tags come back beside the categories, and this time the players put them on: a tag is on a
-- chart while at least one player's vote for it stands (chart_tag_votes, vote 1). A player adds
-- a tag, takes their own vote back (vote 0; the row stays) and reports a tag that does not
-- belong; nobody takes another player's vote away. A manager of the source who upholds a report
-- takes the tag off the chart for good (tag_rejections, kept from 0026_chart_tags).
--
-- The catalog (net.tags) is still the site's, the same for both sources. A manager makes a tag
-- official at once; a player who names a tag the catalog does not have makes a pending one,
-- which becomes official when three different players have used it, on whatever charts.
--
-- The tags that say what a category says already are gone, with whatever 0026's tables still
-- held of them: anime, pops, vsingers, game-original and other-rhythm-games.

DROP TABLE IF EXISTS fanmade.chart_tags, fanmade.tag_suggestions, official.chart_tags, official.tag_suggestions;

ALTER TABLE net.tags
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'official'
        CONSTRAINT tags_status CHECK (status IN ('pending', 'official')),
    -- Who named it; NULL for the tags the site came with and for a player who is gone.
    ADD COLUMN IF NOT EXISTS created_by text REFERENCES net.users (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    DROP CONSTRAINT IF EXISTS tags_names,
    -- A name is 1 to 24 characters with no space at either end.
    ADD CONSTRAINT tags_names CHECK (jsonb_typeof(names) = 'object' AND names ? 'en'
        AND char_length(names ->> 'en') BETWEEN 1 AND 24 AND names ->> 'en' = btrim(names ->> 'en'));

-- A player's tag has the one name they gave it, kept as en (the name shown where a language has
-- none). No two tags share it, whatever the case.
CREATE UNIQUE INDEX IF NOT EXISTS tags_name ON net.tags (lower(names ->> 'en'));
CREATE INDEX IF NOT EXISTS tags_pending ON net.tags (created_by) WHERE status = 'pending';

COMMENT ON TABLE net.tags IS 'The tags a song may have (chart_tag_votes of each source), the same for both sources. official ones are the catalog, in the order shown (position); a pending one was named by a player (created_by) and becomes official once three players have used it.';

DO $$
DECLARE
  src text;
BEGIN
  FOREACH src IN ARRAY ARRAY['fanmade', 'official'] LOOP
    EXECUTE format($sql$
      DELETE FROM %1$I.tag_rejections WHERE tag_id IN ('anime', 'pops', 'vsingers', 'game-original', 'other-rhythm-games');
      -- A tag that is deleted takes what was said of it along.
      ALTER TABLE %1$I.tag_rejections DROP CONSTRAINT IF EXISTS tag_rejections_tag_id_fkey,
          ADD CONSTRAINT tag_rejections_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES net.tags (id) ON DELETE CASCADE;
      COMMENT ON TABLE %1$I.tag_rejections IS 'A tag a manager of the source decided a chart does not have (a report upheld): it is not shown on the chart and votes for it are refused. A manager voting for the tag removes the row.';

      CREATE TABLE IF NOT EXISTS %1$I.chart_tag_votes (
          chart_id text NOT NULL REFERENCES %1$I.charts (id) ON DELETE CASCADE,
          tag_id text NOT NULL REFERENCES net.tags (id) ON DELETE CASCADE,
          user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
          -- 1: the player says the chart has the tag; 0: they took it back.
          vote smallint NOT NULL CONSTRAINT chart_tag_votes_vote CHECK (vote IN (0, 1)),
          created_at timestamptz NOT NULL DEFAULT now(),
          updated_at timestamptz NOT NULL DEFAULT now(),
          PRIMARY KEY (chart_id, tag_id, user_id)
      );
      CREATE INDEX IF NOT EXISTS chart_tag_votes_tag ON %1$I.chart_tag_votes (tag_id, chart_id) WHERE vote = 1;
      CREATE INDEX IF NOT EXISTS chart_tag_votes_user ON %1$I.chart_tag_votes (user_id);
      COMMENT ON TABLE %1$I.chart_tag_votes IS 'A player''s say on whether a chart has a tag (net.tags). The chart has the tag while any vote is 1; a player changes only their own row.';

      CREATE TABLE IF NOT EXISTS %1$I.chart_tag_reports (
          chart_id text NOT NULL REFERENCES %1$I.charts (id) ON DELETE CASCADE,
          tag_id text NOT NULL REFERENCES net.tags (id) ON DELETE CASCADE,
          user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
          reason text NOT NULL DEFAULT '' CONSTRAINT chart_tag_reports_reason CHECK (char_length(reason) <= 500),
          created_at timestamptz NOT NULL DEFAULT now(),
          status text NOT NULL DEFAULT 'pending'
              CONSTRAINT chart_tag_reports_status CHECK (status IN ('pending', 'upheld', 'dismissed')),
          decided_by text REFERENCES net.users (id) ON DELETE SET NULL,
          decided_at timestamptz,
          PRIMARY KEY (chart_id, tag_id, user_id),
          CONSTRAINT chart_tag_reports_decided CHECK ((status = 'pending') = (decided_at IS NULL))
      );
      CREATE INDEX IF NOT EXISTS chart_tag_reports_queue ON %1$I.chart_tag_reports (status, chart_id, tag_id);
      COMMENT ON TABLE %1$I.chart_tag_reports IS 'A player''s report that a tag does not belong on a chart. A manager''s decision answers every waiting report of that tag on that chart: upheld takes the tag off the chart (tag_rejections), dismissed leaves it.';
    $sql$, src);
  END LOOP;
END $$;

DELETE FROM net.tags WHERE id IN ('anime', 'pops', 'vsingers', 'game-original', 'other-rhythm-games');
