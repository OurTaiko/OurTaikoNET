-- Tags: what a song is, beside the categories. A category is the folder of the game's song
-- select a chart is filed in, one bit each and fixed by the game (charts.category_flags); a tag
-- says where a song comes from or what it is (東方Project, BMS, BEMANI…), any number of them on
-- a chart, and is meant for a song select of its own. The tags are the site's, the same for both
-- sources (net.tags), and come with the site: a new one is a new migration.
--
-- Who puts a tag on a chart: a tag manager (TAG_MANAGER), whoever manages the chart's source and
-- the chart's owners set its tags directly. Any other player suggests one (tag_suggestions); a
-- tag manager or the source's manager accepts it, which puts the tag on the chart, or rejects it,
-- which is remembered (tag_rejections). Either answers every suggestion of that tag for that
-- chart: those waiting, and those made later, which are filed already decided.

CREATE TABLE IF NOT EXISTS net.tags (
    id text PRIMARY KEY CONSTRAINT tags_id CHECK (id ~ '^[a-z0-9][a-z0-9-]{0,31}$'),
    -- The tag's name in each of the site's languages; en is the one shown where a language
    -- has none.
    names jsonb NOT NULL CONSTRAINT tags_names CHECK (jsonb_typeof(names) = 'object' AND names ? 'en'),
    position integer NOT NULL CONSTRAINT tags_position UNIQUE
);

COMMENT ON TABLE net.tags IS 'The tags a song may have (chart_tags of each source), in the order shown (position). The same for both sources; a new tag is a new migration.';

INSERT INTO net.tags (id, names, position) VALUES
    ('gekichumai', '{"zh-Hans": "击中萌", "en": "GEKICHUMAI", "ja": "ゲキチュウマイ", "ko": "GEKICHUMAI"}', 1),
    ('touhou', '{"zh-Hans": "东方 PROJECT", "en": "TOUHOU PROJECT", "ja": "東方Project", "ko": "동방 PROJECT"}', 2),
    ('bms', '{"zh-Hans": "BMS", "en": "BMS", "ja": "BMS", "ko": "BMS"}', 3),
    ('anime', '{"zh-Hans": "动漫", "en": "ANIME", "ja": "アニメ", "ko": "애니메이션"}', 4),
    ('pops', '{"zh-Hans": "流行", "en": "POPS", "ja": "ポップス", "ko": "팝"}', 5),
    ('bemani', '{"zh-Hans": "BEMANI", "en": "BEMANI", "ja": "BEMANI", "ko": "BEMANI"}', 6),
    ('vsingers', '{"zh-Hans": "虚拟歌姬", "en": "VSINGERS", "ja": "ボーカロイド", "ko": "버추얼 싱어"}', 7),
    ('game-original', '{"zh-Hans": "游戏原声带", "en": "GAME ORIGINAL", "ja": "ゲームミュージック", "ko": "게임 음악"}', 8),
    ('other-rhythm-games', '{"zh-Hans": "其他音乐游戏", "en": "OTHER RHYTHM GAMES", "ja": "その他の音楽ゲーム", "ko": "기타 리듬 게임"}', 9),
    ('album', '{"zh-Hans": "专辑曲", "en": "ALBUM", "ja": "アルバム曲", "ko": "앨범 수록곡"}', 10),
    ('internet', '{"zh-Hans": "网络 / 梗曲", "en": "INTERNET", "ja": "ネット・ミーム", "ko": "인터넷 / 밈"}', 11),
    ('project-sekai', '{"zh-Hans": "世界计划", "en": "PROJECT SEKAI", "ja": "プロジェクトセカイ", "ko": "프로젝트 세카이"}', 12)
ON CONFLICT (id) DO NOTHING;

-- The same three tables in each source.
DO $$
DECLARE
  src text;
BEGIN
  FOREACH src IN ARRAY ARRAY['fanmade', 'official'] LOOP
    EXECUTE format($sql$
      CREATE TABLE IF NOT EXISTS %1$I.chart_tags (
          chart_id text NOT NULL REFERENCES %1$I.charts (id) ON DELETE CASCADE,
          tag_id text NOT NULL REFERENCES net.tags (id),
          -- Who put it on: the tag manager who accepted a suggestion, or whoever set it directly.
          added_by text REFERENCES net.users (id) ON DELETE SET NULL,
          added_at timestamptz NOT NULL DEFAULT now(),
          PRIMARY KEY (chart_id, tag_id)
      );
      CREATE INDEX IF NOT EXISTS chart_tags_tag ON %1$I.chart_tags (tag_id, chart_id);
      COMMENT ON TABLE %1$I.chart_tags IS 'The tags (net.tags) each chart has.';

      CREATE TABLE IF NOT EXISTS %1$I.tag_rejections (
          chart_id text NOT NULL REFERENCES %1$I.charts (id) ON DELETE CASCADE,
          tag_id text NOT NULL REFERENCES net.tags (id),
          rejected_by text REFERENCES net.users (id) ON DELETE SET NULL,
          rejected_at timestamptz NOT NULL DEFAULT now(),
          PRIMARY KEY (chart_id, tag_id)
      );
      COMMENT ON TABLE %1$I.tag_rejections IS 'A tag a tag manager decided a chart does not have: later suggestions of it are filed rejected. Putting the tag on the chart removes the row.';

      CREATE TABLE IF NOT EXISTS %1$I.tag_suggestions (
          chart_id text NOT NULL REFERENCES %1$I.charts (id) ON DELETE CASCADE,
          tag_id text NOT NULL REFERENCES net.tags (id),
          user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
          created_at timestamptz NOT NULL DEFAULT now(),
          status text NOT NULL DEFAULT 'pending'
              CONSTRAINT tag_suggestions_status CHECK (status IN ('pending', 'accepted', 'rejected')),
          -- The tag manager who decided; NULL for a suggestion filed after the decision.
          decided_by text REFERENCES net.users (id) ON DELETE SET NULL,
          decided_at timestamptz,
          PRIMARY KEY (chart_id, tag_id, user_id),
          CONSTRAINT tag_suggestions_decided CHECK ((status = 'pending') = (decided_at IS NULL))
      );
      -- The tag managers' queue, and what was decided.
      CREATE INDEX IF NOT EXISTS tag_suggestions_queue ON %1$I.tag_suggestions (status, chart_id, tag_id);
      CREATE INDEX IF NOT EXISTS tag_suggestions_user ON %1$I.tag_suggestions (user_id);
      COMMENT ON TABLE %1$I.tag_suggestions IS 'A player''s suggestion that a chart has a tag. A tag manager''s decision answers every suggestion of that tag for that chart, and a suggestion made after it is filed with it (accepted when the chart has the tag, rejected when tag_rejections has it).';
    $sql$, src);
  END LOOP;
END $$;
