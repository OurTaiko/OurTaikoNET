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

-- A retired category and the tag it becomes.
CREATE TEMPORARY TABLE category_tags (bit integer PRIMARY KEY, tag_id text NOT NULL) ON COMMIT DROP;
INSERT INTO category_tags VALUES (64, 'touhou'), (128, 'gekichumai'), (2048, 'bms'), (8192, 'internet');

-- A chart is in the category, so whatever once said it has not the tag is answered.
DELETE FROM fanmade.tag_rejections r USING fanmade.charts c, category_tags t
WHERE r.chart_id = c.id AND r.tag_id = t.tag_id AND c.category_flags & t.bit <> 0;

-- The tag is the vote of who put the chart there: its first owner, and the players whose
-- suggestion of the category waits or was accepted.
INSERT INTO fanmade.chart_tag_votes (chart_id, tag_id, user_id, vote)
SELECT c.id, t.tag_id, c.owner_ids[1], 1 FROM fanmade.charts c JOIN category_tags t ON c.category_flags & t.bit <> 0
ON CONFLICT (chart_id, tag_id, user_id) DO UPDATE SET vote = 1, updated_at = now();

INSERT INTO fanmade.chart_tag_votes (chart_id, tag_id, user_id, vote, created_at)
SELECT s.chart_id, t.tag_id, s.user_id, 1, s.created_at
FROM fanmade.category_suggestions s JOIN category_tags t ON t.bit = s.category_flag
WHERE s.status IN ('pending', 'accepted')
  AND NOT EXISTS (SELECT 1 FROM fanmade.category_rejections r WHERE r.chart_id = s.chart_id AND r.category_flag = s.category_flag)
ON CONFLICT (chart_id, tag_id, user_id) DO UPDATE SET vote = 1, updated_at = now();

-- A manager's decision that a chart is not in the category is the decision that it has not the tag.
INSERT INTO fanmade.tag_rejections (chart_id, tag_id, rejected_by, rejected_at)
SELECT r.chart_id, t.tag_id, r.rejected_by, r.rejected_at FROM fanmade.category_rejections r JOIN category_tags t ON t.bit = r.category_flag
ON CONFLICT (chart_id, tag_id) DO NOTHING;

-- A pending tag these votes brought to three players is official, as a vote would have made it.
UPDATE net.tags g SET status = 'official'
WHERE g.status = 'pending' AND (SELECT count(DISTINCT user_id) FROM (
    SELECT user_id FROM fanmade.chart_tag_votes WHERE tag_id = g.id AND vote = 1
    UNION SELECT user_id FROM official.chart_tag_votes WHERE tag_id = g.id AND vote = 1) u) >= 3;

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

-- A suggestion of gekichumai, bemani, project-sekai or bms that waits is one of rhythm-game, for
-- a chart that is not in it yet; the suggestions and decisions of retired categories go.
INSERT INTO fanmade.category_suggestions (chart_id, category_flag, user_id, created_at)
SELECT s.chart_id, 1024, s.user_id, min(s.created_at)
FROM fanmade.category_suggestions s JOIN fanmade.charts c ON c.id = s.chart_id
WHERE s.status = 'pending' AND s.category_flag & 2944 <> 0 AND c.category_flags & 1024 = 0
GROUP BY s.chart_id, s.user_id
ON CONFLICT (chart_id, category_flag, user_id) DO NOTHING;
DELETE FROM fanmade.category_suggestions WHERE category_flag & 15296 <> 0;
DELETE FROM fanmade.category_rejections WHERE category_flag & 15296 <> 0;

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
ALTER TABLE fanmade.playlists DROP CONSTRAINT IF EXISTS playlists_category_flag_check,
    ADD CONSTRAINT playlists_category_flag_check CHECK (category_flag IN (1, 2, 4, 8, 16, 32, 1024, 16384));
