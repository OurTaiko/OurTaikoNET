-- Fanmade's categories go from fourteen to eight, one for each of the official source's: what
-- 0027_fanmade_categories added says where a song comes from, and that is a tag's business since
-- 0031_tag_votes. The eight, by their bits (internal/songs/httpapi/sources.go):
--
--   game 1 (games that are not rhythm games), vocaloid 2 (was virtual-singer), pop 4,
--   classics 8 (was classic), variety 16, anime 32, rhythm-game 1024 (was other-rhythm-games;
--   it stands where the official source has namco), kids 16384 (new).
--
-- The bits 64, 128, 256, 512, 2048, 4096 and 8192 are retired and never to mean anything else.
-- What becomes of a chart in a retired category:
--
--   touhou 64      -> variety, and the tag touhou
--   gekichumai 128 -> rhythm-game, and the tag gekichumai
--   bms 2048       -> rhythm-game, and the tag bms
--   bemani 256, project-sekai 512 -> rhythm-game
--   internet 8192  -> the tag internet; variety when it is in no other category
--   album 4096     -> variety when it is in no other category
--
-- and a chart in rhythm-game leaves game, which is the games that are not rhythm games now.
--
-- Players no longer suggest categories (0027's category_suggestions and category_rejections):
-- what a player says of a song is a tag now. Nothing reads those two tables; they are left for
-- a later migration to drop, because the release before this one reads them while it is replaced.

-- A retired category and the tag it becomes.
CREATE TEMPORARY TABLE category_tags (bit integer PRIMARY KEY, tag_id text NOT NULL) ON COMMIT DROP;
INSERT INTO category_tags VALUES (64, 'touhou'), (128, 'gekichumai'), (2048, 'bms'), (8192, 'internet');

-- A chart is in the category, so whatever once said it has not the tag is answered.
DELETE FROM fanmade.tag_rejections r USING fanmade.charts c, category_tags t
WHERE r.chart_id = c.id AND r.tag_id = t.tag_id AND c.category_flags & t.bit <> 0;

-- The tag is the vote of who put the chart there: its first owner.
INSERT INTO fanmade.chart_tag_votes (chart_id, tag_id, user_id, vote)
SELECT c.id, t.tag_id, c.owner_ids[1], 1 FROM fanmade.charts c JOIN category_tags t ON c.category_flags & t.bit <> 0
ON CONFLICT (chart_id, tag_id, user_id) DO UPDATE SET vote = 1, updated_at = now();

-- The charts. 2944 is gekichumai, bemani, project-sekai and bms; 3968 those and rhythm-game;
-- 15296 every retired bit. A chart left in no category (it was in album or internet alone) is in
-- Variety, which is where a chart is when it has nowhere else to be.
UPDATE fanmade.charts c SET category_flags = CASE WHEN n.flags = 0 THEN 16 ELSE n.flags END
FROM (SELECT id, (category_flags
          | CASE WHEN category_flags & 2944 <> 0 THEN 1024 ELSE 0 END
          | CASE WHEN category_flags & 64 <> 0 THEN 16 ELSE 0 END)
        & ~CASE WHEN category_flags & 3968 <> 0 THEN 1 ELSE 0 END
        & ~15296 AS flags
      FROM fanmade.charts
      WHERE category_flags & 15296 <> 0 OR category_flags & 1025 = 1025) n
WHERE c.id = n.id;

-- A playlist is filed under one category.
UPDATE fanmade.playlists SET category_flag = CASE WHEN category_flag & 2944 <> 0 THEN 1024 ELSE 16 END
WHERE category_flag & 15296 <> 0;

-- A reward rule for clearing a category of Fanmade: of the four that became rhythm-game it is
-- of rhythm-game, where no rule says the same already; of touhou, album or internet it has no
-- category left to be of and goes (what it gave stays given).
DELETE FROM net.reward_rules r
WHERE r.source = 'fanmade' AND r.category_flag & 2944 <> 0 AND EXISTS (
    SELECT 1 FROM net.reward_rules o
    WHERE o.id <> r.id AND o.source = 'fanmade' AND (o.category_flag = 1024 OR (o.category_flag & 2944 <> 0 AND o.id < r.id))
      AND (o.item, o.title_id, o.don_part, o.don_item, o.kind, o.chart_id, o.dan_id, o.course, o.clear, o.count)
          IS NOT DISTINCT FROM (r.item, r.title_id, r.don_part, r.don_item, r.kind, r.chart_id, r.dan_id, r.course, r.clear, r.count));
UPDATE net.reward_rules SET category_flag = 1024 WHERE source = 'fanmade' AND category_flag & 2944 <> 0;
DELETE FROM net.reward_rules WHERE source = 'fanmade' AND category_flag & 15296 <> 0;

-- 17471 is the eight bits.
ALTER TABLE fanmade.charts DROP CONSTRAINT IF EXISTS charts_category_flags_check,
    ADD CONSTRAINT charts_category_flags_check CHECK (category_flags >= 0 AND (category_flags & 17471) = category_flags);
COMMENT ON TABLE fanmade.category_suggestions IS 'Unread since 0032_fanmade_eight_categories (players no longer suggest categories); to be dropped.';
COMMENT ON TABLE fanmade.category_rejections IS 'Unread since 0032_fanmade_eight_categories (players no longer suggest categories); to be dropped.';
COMMENT ON TABLE official.category_suggestions IS 'Unread since 0032_fanmade_eight_categories (players no longer suggest categories); to be dropped.';
COMMENT ON TABLE official.category_rejections IS 'Unread since 0032_fanmade_eight_categories (players no longer suggest categories); to be dropped.';
ALTER TABLE fanmade.playlists DROP CONSTRAINT IF EXISTS playlists_category_flag_check,
    ADD CONSTRAINT playlists_category_flag_check CHECK (category_flag IN (1, 2, 4, 8, 16, 32, 1024, 16384));
