-- The 真打 (shinuchi) scoring of each chart block, as TJAParser reads it from the stored file
-- (inspect "scoring", v0.18.0 or newer): what a 良, a 可 and a drumroll or balloon hit are worth,
-- and the kiwami line. A score is then the site's, worked out from the counts the game sent:
--   (good + Ad-Lib 良) × good_points + (ok + Ad-Lib 可) × ok_points + drumroll × long_hit_points
-- scoring_version is the rule the points were read with (tja.ScoringVersion); NULL until the
-- block has been read for it, so blocks from before this column are filled at startup. With
-- the version set, NULL points mean the tool cannot score the block, and its scores keep what
-- the game sent.
ALTER TABLE fanmade.chart_difficulties
    ADD COLUMN IF NOT EXISTS good_points integer,
    ADD COLUMN IF NOT EXISTS ok_points integer,
    ADD COLUMN IF NOT EXISTS long_hit_points integer,
    ADD COLUMN IF NOT EXISTS kiwami_threshold integer,
    ADD COLUMN IF NOT EXISTS scoring_version smallint,
    DROP CONSTRAINT IF EXISTS chart_difficulties_scoring_check,
    ADD CONSTRAINT chart_difficulties_scoring_check CHECK (
        (good_points IS NULL AND ok_points IS NULL AND long_hit_points IS NULL AND kiwami_threshold IS NULL)
        OR (scoring_version IS NOT NULL AND good_points >= 0 AND ok_points >= 0 AND long_hit_points >= 0 AND kiwami_threshold >= 0));
ALTER TABLE official.chart_difficulties
    ADD COLUMN IF NOT EXISTS good_points integer,
    ADD COLUMN IF NOT EXISTS ok_points integer,
    ADD COLUMN IF NOT EXISTS long_hit_points integer,
    ADD COLUMN IF NOT EXISTS kiwami_threshold integer,
    ADD COLUMN IF NOT EXISTS scoring_version smallint,
    DROP CONSTRAINT IF EXISTS chart_difficulties_scoring_check,
    ADD CONSTRAINT chart_difficulties_scoring_check CHECK (
        (good_points IS NULL AND ok_points IS NULL AND long_hit_points IS NULL AND kiwami_threshold IS NULL)
        OR (scoring_version IS NOT NULL AND good_points >= 0 AND ok_points >= 0 AND long_hit_points >= 0 AND kiwami_threshold >= 0));
COMMENT ON COLUMN fanmade.chart_difficulties.good_points IS 'Points of a 良 (TJAParser ShinuchiScoring); NULL while the block is not scored.';
COMMENT ON COLUMN official.chart_difficulties.good_points IS 'Points of a 良 (TJAParser ShinuchiScoring); NULL while the block is not scored.';
COMMENT ON COLUMN fanmade.chart_difficulties.scoring_version IS 'The scoring rule the points were read with; NULL while the block has not been read for it.';
COMMENT ON COLUMN official.chart_difficulties.scoring_version IS 'The scoring rule the points were read with; NULL while the block has not been read for it.';

-- The one place the formula is written: the insert trigger and the rescore below use it, and the
-- game's ShinuchiScore adds up the same.
CREATE OR REPLACE FUNCTION net.shinuchi_score(good integer, ok integer, drumroll integer, adlib integer[],
    good_points integer, ok_points integer, long_hit_points integer) RETURNS bigint
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT (good::bigint + COALESCE(adlib[1], 0)) * good_points
  + (ok::bigint + COALESCE(adlib[2], 0)) * ok_points
  + drumroll::bigint * long_hit_points
$$;

-- A score's rank against its block's kiwami line: 0 none; 1–6 白粋, 銅粋, 銀粋, 金粋, 桃雅, 紫雅 at
-- 50, 60, 70, 80, 90 and 95 % of the line (no rounding: score × 100 ≥ line × percent); 7 極 at the
-- line itself. NULL when the block has no line.
CREATE OR REPLACE FUNCTION net.score_rank(score bigint, kiwami integer) RETURNS smallint
    LANGUAGE sql IMMUTABLE
    AS $$
 SELECT (CASE WHEN kiwami IS NULL OR score IS NULL THEN NULL
  WHEN score >= kiwami THEN 7
  WHEN score * 100 >= kiwami::bigint * 95 THEN 6
  WHEN score * 100 >= kiwami::bigint * 90 THEN 5
  WHEN score * 100 >= kiwami::bigint * 80 THEN 4
  WHEN score * 100 >= kiwami::bigint * 70 THEN 3
  WHEN score * 100 >= kiwami::bigint * 60 THEN 2
  WHEN score * 100 >= kiwami::bigint * 50 THEN 1
  ELSE 0 END)::smallint
$$;

-- The rank of a score worked out by the site; NULL for one whose block is not scored, or that
-- was played on content since replaced.
ALTER TABLE net.scores ADD COLUMN IF NOT EXISTS score_rank smallint
    CONSTRAINT scores_score_rank_check CHECK (score_rank BETWEEN 0 AND 7);
COMMENT ON COLUMN net.scores.score_rank IS 'Rank against the block''s kiwami line (net.score_rank); NULL while the block is not scored.';

-- A new score takes the site's value and rank when its block is scored, whatever the game sent:
-- an older game scores by an older rule.
CREATE OR REPLACE FUNCTION fanmade.scores_shinuchi() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
DECLARE scored bigint; kiwami integer;
BEGIN
 SELECT net.shinuchi_score(NEW.good, NEW.ok, NEW.drumroll, NEW.adlib, d.good_points, d.ok_points, d.long_hit_points), d.kiwami_threshold
  INTO scored, kiwami FROM chart_difficulties d WHERE d.id=NEW.difficulty_id;
 IF scored IS NOT NULL THEN NEW.score := scored; END IF;
 NEW.score_rank := CASE WHEN scored IS NULL THEN NULL ELSE net.score_rank(scored, kiwami) END;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION official.scores_shinuchi() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
DECLARE scored bigint; kiwami integer;
BEGIN
 SELECT net.shinuchi_score(NEW.good, NEW.ok, NEW.drumroll, NEW.adlib, d.good_points, d.ok_points, d.long_hit_points), d.kiwami_threshold
  INTO scored, kiwami FROM chart_difficulties d WHERE d.id=NEW.difficulty_id;
 IF scored IS NOT NULL THEN NEW.score := scored; END IF;
 NEW.score_rank := CASE WHEN scored IS NULL THEN NULL ELSE net.score_rank(scored, kiwami) END;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS scores_shinuchi ON fanmade.scores;
CREATE TRIGGER scores_shinuchi BEFORE INSERT ON fanmade.scores FOR EACH ROW EXECUTE FUNCTION fanmade.scores_shinuchi();
DROP TRIGGER IF EXISTS scores_shinuchi ON official.scores;
CREATE TRIGGER scores_shinuchi BEFORE INSERT ON official.scores FOR EACH ROW EXECUTE FUNCTION official.scores_shinuchi();

-- New points for the same chart content (the block read for the first time, or by a newer rule)
-- rescore its current scores and their ranks. A replaced block (another content_hash) does not: its scores were
-- played on the old content and the retire trigger takes them out of the latest.
CREATE OR REPLACE FUNCTION fanmade.chart_difficulties_rescore() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
 UPDATE scores SET score=net.shinuchi_score(good, ok, drumroll, adlib, NEW.good_points, NEW.ok_points, NEW.long_hit_points),
  score_rank=net.score_rank(net.shinuchi_score(good, ok, drumroll, adlib, NEW.good_points, NEW.ok_points, NEW.long_hit_points), NEW.kiwami_threshold)
  WHERE is_latest AND difficulty_id=NEW.id;
 RETURN NULL;
END $$;
CREATE OR REPLACE FUNCTION official.chart_difficulties_rescore() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
 UPDATE scores SET score=net.shinuchi_score(good, ok, drumroll, adlib, NEW.good_points, NEW.ok_points, NEW.long_hit_points),
  score_rank=net.score_rank(net.shinuchi_score(good, ok, drumroll, adlib, NEW.good_points, NEW.ok_points, NEW.long_hit_points), NEW.kiwami_threshold)
  WHERE is_latest AND difficulty_id=NEW.id;
 RETURN NULL;
END $$;
DROP TRIGGER IF EXISTS chart_difficulties_rescore ON fanmade.chart_difficulties;
CREATE TRIGGER chart_difficulties_rescore AFTER UPDATE OF good_points, ok_points, long_hit_points, kiwami_threshold ON fanmade.chart_difficulties
    FOR EACH ROW WHEN (new.good_points IS NOT NULL AND old.content_hash IS NOT DISTINCT FROM new.content_hash
        AND (old.good_points, old.ok_points, old.long_hit_points, old.kiwami_threshold) IS DISTINCT FROM (new.good_points, new.ok_points, new.long_hit_points, new.kiwami_threshold))
    EXECUTE FUNCTION fanmade.chart_difficulties_rescore();
DROP TRIGGER IF EXISTS chart_difficulties_rescore ON official.chart_difficulties;
CREATE TRIGGER chart_difficulties_rescore AFTER UPDATE OF good_points, ok_points, long_hit_points, kiwami_threshold ON official.chart_difficulties
    FOR EACH ROW WHEN (new.good_points IS NOT NULL AND old.content_hash IS NOT DISTINCT FROM new.content_hash
        AND (old.good_points, old.ok_points, old.long_hit_points, old.kiwami_threshold) IS DISTINCT FROM (new.good_points, new.ok_points, new.long_hit_points, new.kiwami_threshold))
    EXECUTE FUNCTION official.chart_difficulties_rescore();

-- The scoring repeats what the stored file says, so reading it does not change the chart: the
-- chart's update_at follows only the columns that describe the block.
DROP TRIGGER IF EXISTS chart_difficulties_touch_chart ON fanmade.chart_difficulties;
CREATE TRIGGER chart_difficulties_touch_chart AFTER INSERT OR DELETE OR UPDATE OF chart_id, course, side, level, branching, maker, content_hash
    ON fanmade.chart_difficulties FOR EACH ROW EXECUTE FUNCTION fanmade.touch_related_chart();
DROP TRIGGER IF EXISTS chart_difficulties_touch_chart ON official.chart_difficulties;
CREATE TRIGGER chart_difficulties_touch_chart AFTER INSERT OR DELETE OR UPDATE OF chart_id, course, side, level, branching, maker, content_hash
    ON official.chart_difficulties FOR EACH ROW EXECUTE FUNCTION official.touch_related_chart();
