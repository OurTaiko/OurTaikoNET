--
-- PostgreSQL database dump
--


-- Dumped from database version 18.6 (Homebrew)
-- Dumped by pg_dump version 18.6 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: fanmade; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA fanmade;


--
-- Name: SCHEMA fanmade; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON SCHEMA fanmade IS 'Song source fanmade: its charts, files, dan courses and what players wrote about them.';


--
-- Name: net; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA net;


--
-- Name: SCHEMA net; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON SCHEMA net IS 'Shared by both song sources: players and their groups, sign-in, titles, and one parent table over each source''s scores and dan attempts.';


--
-- Name: official; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA official;


--
-- Name: SCHEMA official; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON SCHEMA official IS 'Song source official: its charts, files, dan courses and what players wrote about them.';


--
-- Name: chart_difficulties_invalidate_dan_songs(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.chart_difficulties_invalidate_dan_songs() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
 UPDATE dan_course_songs SET valid=false WHERE chart_id=OLD.chart_id AND course=OLD.course AND side=OLD.side;
 RETURN NULL;
END $$;


--
-- Name: chart_difficulties_retire_scores(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.chart_difficulties_retire_scores() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
 UPDATE scores SET is_latest=false WHERE is_latest AND difficulty_id=NEW.id;
 UPDATE dan_course_songs SET valid=false WHERE chart_id=NEW.chart_id AND course=NEW.course AND side=NEW.side;
 RETURN NULL;
END $$;


--
-- Name: chart_resources_touch_charts(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.chart_resources_touch_charts() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
  IF NEW IS DISTINCT FROM OLD THEN
    UPDATE charts SET update_at = greatest(update_at, clock_timestamp())
     WHERE NEW.id IN (tja_file, audio_file, cover_file, movie_file, bgimage_file);
  END IF;
  RETURN NULL;
END;
$$;


--
-- Name: charts_touch_update_at(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.charts_touch_update_at() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
  IF to_jsonb(NEW) - 'update_at' - 'preview_file' IS DISTINCT FROM to_jsonb(OLD) - 'update_at' - 'preview_file' THEN
    NEW.update_at := greatest(OLD.update_at, clock_timestamp());
  ELSE
    -- Only update_at or the preview clip was written: a related row touching the chart.
    NEW.update_at := greatest(OLD.update_at, NEW.update_at);
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: check_chart_owners(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.check_chart_owners() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
DECLARE owner text;
BEGIN
 FOREACH owner IN ARRAY NEW.owner_ids LOOP
  PERFORM id FROM users WHERE id=owner FOR KEY SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'chart owner does not exist' USING ERRCODE='23503'; END IF;
 END LOOP;
 RETURN NEW;
END;
$$;


--
-- Name: comment_confidence(integer, integer); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.comment_confidence(up integer, down integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    SET search_path TO 'fanmade', 'net'
    AS $$
 SELECT CASE WHEN up+down=0 THEN 0 ELSE
  (up::float8/(up+down) + 1.642374415149/(2*(up+down))
   - 1.281551565545*sqrt((up::float8/(up+down)*(1-up::float8/(up+down)) + 1.642374415149/(4*(up+down)))/(up+down)))
  / (1 + 1.642374415149/(up+down)) END
$$;


--
-- Name: comment_controversy(integer, integer); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.comment_controversy(up integer, down integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    SET search_path TO 'fanmade', 'net'
    AS $$
 SELECT CASE WHEN up<=0 OR down<=0 THEN 0
  ELSE power((up+down)::float8, CASE WHEN up>down THEN down::float8/up ELSE up::float8/down END) END
$$;


--
-- Name: count_chart_vote(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.count_chart_vote() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
DECLARE song text; up integer:=0; down integer:=0;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  song:=OLD.chart_id;
  IF OLD.value=1 THEN up:=up-1; ELSE down:=down-1; END IF;
 END IF;
 IF TG_OP IN ('INSERT','UPDATE') THEN
  song:=NEW.chart_id;
  IF NEW.value=1 THEN up:=up+1; ELSE down:=down+1; END IF;
 END IF;
 -- A cascading song deletion has already removed the song row.
 IF EXISTS(SELECT 1 FROM charts WHERE id=song) THEN
  -- CHECK applies to the proposed row before ON CONFLICT, so never propose a
  -- negative count; a decrement always finds the existing row.
  INSERT INTO chart_stats(chart_id,upvotes,downvotes) VALUES(song,greatest(up,0),greatest(down,0))
  ON CONFLICT(chart_id) DO UPDATE SET upvotes=chart_stats.upvotes+up,downvotes=chart_stats.downvotes+down;
 END IF;
 RETURN NULL;
END;
$$;


--
-- Name: count_comment(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.count_comment() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
DECLARE delta integer; rec comments;
BEGIN
 IF TG_OP='INSERT' THEN delta:=1; rec:=NEW; ELSE delta:=-1; rec:=OLD; END IF;
 IF rec.parent_id IS NOT NULL THEN
  UPDATE comments SET reply_count=reply_count+delta WHERE id=rec.parent_id;
 END IF;
 IF rec.dan_id IS NOT NULL THEN
  UPDATE dan_courses SET comment_count=comment_count+delta WHERE id=rec.dan_id;
 ELSIF EXISTS(SELECT 1 FROM charts WHERE id=rec.chart_id) THEN
  INSERT INTO chart_stats(chart_id,comment_count) VALUES(rec.chart_id,greatest(delta,0))
  ON CONFLICT(chart_id) DO UPDATE SET comment_count=chart_stats.comment_count+delta;
 END IF;
 RETURN NULL;
END;
$$;


--
-- Name: count_comment_vote(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.count_comment_vote() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
DECLARE target text; up integer:=0; down integer:=0;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  target:=OLD.comment_id;
  IF OLD.value=1 THEN up:=up-1; ELSE down:=down-1; END IF;
 END IF;
 IF TG_OP IN ('INSERT','UPDATE') THEN
  target:=NEW.comment_id;
  IF NEW.value=1 THEN up:=up+1; ELSE down:=down+1; END IF;
 END IF;
 UPDATE comments SET upvotes=upvotes+up,downvotes=downvotes+down WHERE id=target;
 RETURN NULL;
END;
$$;


--
-- Name: count_dan_vote(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.count_dan_vote() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
DECLARE target text; up integer:=0; down integer:=0;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  target:=OLD.dan_id;
  IF OLD.value=1 THEN up:=up-1; ELSE down:=down-1; END IF;
 END IF;
 IF TG_OP IN ('INSERT','UPDATE') THEN
  target:=NEW.dan_id;
  IF NEW.value=1 THEN up:=up+1; ELSE down:=down+1; END IF;
 END IF;
 UPDATE dan_courses SET upvotes=upvotes+up,downvotes=downvotes+down WHERE id=target;
 RETURN NULL;
END;
$$;


--
-- Name: dan_attempt_scores(text); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.dan_attempt_scores(attempt text) RETURNS text[]
    LANGUAGE sql STABLE
    SET search_path TO 'fanmade', 'net'
    AS $$
 SELECT COALESCE(array_agg(m.id ORDER BY g.position), '{}')
 FROM dan_attempts a
 CROSS JOIN LATERAL generate_series(0, cardinality(a.songs_played) - 1) g(position)
 LEFT JOIN LATERAL (
  SELECT sc.id FROM dan_course_songs s
  JOIN chart_difficulties d ON d.chart_id = s.chart_id AND d.course = s.course
  JOIN scores sc ON sc.difficulty_id = d.id AND sc.user_id = a.user_id
  WHERE s.course_id = a.course_id AND s.position = g.position
   AND sc.submitted_at <= a.submitted_at + interval '30 seconds'
   AND sc.submitted_at > GREATEST(a.submitted_at - interval '1 hour',
    COALESCE((SELECT max(b.submitted_at) FROM dan_attempts b WHERE b.user_id = a.user_id AND b.submitted_at < a.submitted_at) + interval '30 seconds', '-infinity'))
  ORDER BY sc.submitted_at DESC, sc.id LIMIT 1
 ) m ON true
 WHERE a.id = attempt
$$;


--
-- Name: default_demo_end(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.default_demo_end() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
 IF NEW.demo_end IS NULL THEN NEW.demo_end := NEW.demo_start+15; END IF;
 RETURN NEW;
END;
$$;


--
-- Name: protect_chart_owner(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.protect_chart_owner() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.id=OLD.id THEN RETURN NEW; END IF;
 IF EXISTS(SELECT 1 FROM charts WHERE owner_ids @> ARRAY[OLD.id]) THEN
  RAISE EXCEPTION 'user owns charts' USING ERRCODE='23503';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;


--
-- Name: retire_chart_files(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.retire_chart_files() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
 INSERT INTO retired_files(storage_key) SELECT storage_key FROM chart_resources
  WHERE id IN (OLD.tja_file,OLD.audio_file,OLD.cover_file,OLD.preview_file,OLD.movie_file,OLD.bgimage_file)
   AND (TG_OP='DELETE' OR NOT id=ANY(array_remove(ARRAY[NEW.tja_file,NEW.audio_file,NEW.cover_file,NEW.preview_file,NEW.movie_file,NEW.bgimage_file],NULL)))
  ON CONFLICT DO NOTHING;
 RETURN NULL;
END;
$$;


--
-- Name: retire_playlist_cover(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.retire_playlist_cover() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
 INSERT INTO retired_files(storage_key) SELECT storage_key FROM chart_resources
  WHERE id IN (OLD.cover_file,OLD.game_cover_file)
   AND (TG_OP='DELETE' OR NOT id=ANY(array_remove(ARRAY[NEW.cover_file,NEW.game_cover_file],NULL)))
  ON CONFLICT DO NOTHING;
 RETURN NULL;
END;
$$;


--
-- Name: touch_related_chart(); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.touch_related_chart() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
DECLARE target text;
BEGIN
  IF TG_OP = 'DELETE' THEN target := OLD.chart_id; ELSE target := NEW.chart_id; END IF;
  UPDATE charts SET update_at = greatest(update_at, clock_timestamp()) WHERE id = target;
  RETURN NULL;
END;
$$;


--
-- Name: valid_chart_owners(text[]); Type: FUNCTION; Schema: fanmade; Owner: -
--

CREATE FUNCTION fanmade.valid_chart_owners(ids text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    SET search_path TO 'fanmade', 'net'
    AS $$
 SELECT COALESCE(array_ndims(ids)=1 AND array_lower(ids,1)=1 AND cardinality(ids)>0
  AND cardinality(ids)=(SELECT count(DISTINCT id) FROM unnest(ids) id WHERE id<>''),false)
$$;


--
-- Name: ensure_player(); Type: FUNCTION; Schema: net; Owner: -
--

CREATE FUNCTION net.ensure_player() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'net'
    AS $$
DECLARE
  player text := to_jsonb(NEW) ->> TG_ARGV[0];
BEGIN
  -- Looked up first: nearly always the player is there already and nothing is written.
  IF player IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users WHERE id = player) THEN
    INSERT INTO users (id) VALUES (player) ON CONFLICT (id) DO NOTHING;
  END IF;
  RETURN NEW;
END $$;


--
-- Name: chart_difficulties_invalidate_dan_songs(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.chart_difficulties_invalidate_dan_songs() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
  UPDATE official.dan_course_songs SET valid = false WHERE chart_id = OLD.chart_id AND course = OLD.course AND side = OLD.side;
  RETURN NULL;
END;
$$;


--
-- Name: chart_difficulties_retire_scores(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.chart_difficulties_retire_scores() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
  UPDATE official.scores SET is_latest = false WHERE is_latest AND difficulty_id = NEW.id;
  UPDATE official.dan_course_songs SET valid = false WHERE chart_id = NEW.chart_id AND course = NEW.course AND side = NEW.side;
  RETURN NULL;
END;
$$;


--
-- Name: chart_resources_touch_charts(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.chart_resources_touch_charts() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
  IF NEW IS DISTINCT FROM OLD THEN
    UPDATE official.charts SET update_at = greatest(update_at, clock_timestamp())
     WHERE NEW.id IN (tja_file, audio_file, cover_file, movie_file, bgimage_file);
  END IF;
  RETURN NULL;
END;
$$;


--
-- Name: charts_retire_scores(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.charts_retire_scores() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
  UPDATE official.scores SET is_latest = false
   WHERE is_latest AND difficulty_id IN (SELECT id FROM official.chart_difficulties WHERE chart_id = NEW.id);
  RETURN NULL;
END;
$$;


--
-- Name: charts_touch_update_at(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.charts_touch_update_at() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
  IF to_jsonb(NEW) - 'update_at' - 'preview_file' IS DISTINCT FROM to_jsonb(OLD) - 'update_at' - 'preview_file' THEN
    NEW.update_at := greatest(OLD.update_at, clock_timestamp());
  ELSE
    -- Only update_at or the preview clip was written: a related row touching the chart.
    NEW.update_at := greatest(OLD.update_at, NEW.update_at);
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: check_chart_owners(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.check_chart_owners() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
DECLARE owner text;
BEGIN
 FOREACH owner IN ARRAY NEW.owner_ids LOOP
  PERFORM id FROM users WHERE id=owner FOR KEY SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'chart owner does not exist' USING ERRCODE='23503'; END IF;
 END LOOP;
 RETURN NEW;
END;
$$;


--
-- Name: comment_confidence(integer, integer); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.comment_confidence(up integer, down integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    SET search_path TO 'official', 'net'
    AS $$
 SELECT CASE WHEN up+down=0 THEN 0 ELSE
  (up::float8/(up+down) + 1.642374415149/(2*(up+down))
   - 1.281551565545*sqrt((up::float8/(up+down)*(1-up::float8/(up+down)) + 1.642374415149/(4*(up+down)))/(up+down)))
  / (1 + 1.642374415149/(up+down)) END
$$;


--
-- Name: comment_controversy(integer, integer); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.comment_controversy(up integer, down integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    SET search_path TO 'official', 'net'
    AS $$
 SELECT CASE WHEN up<=0 OR down<=0 THEN 0
  ELSE power((up+down)::float8, CASE WHEN up>down THEN down::float8/up ELSE up::float8/down END) END
$$;


--
-- Name: count_chart_vote(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.count_chart_vote() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
DECLARE song text; up integer:=0; down integer:=0;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  song:=OLD.chart_id;
  IF OLD.value=1 THEN up:=up-1; ELSE down:=down-1; END IF;
 END IF;
 IF TG_OP IN ('INSERT','UPDATE') THEN
  song:=NEW.chart_id;
  IF NEW.value=1 THEN up:=up+1; ELSE down:=down+1; END IF;
 END IF;
 -- A cascading chart deletion has already removed the chart row.
 IF EXISTS(SELECT 1 FROM charts WHERE id=song) THEN
  -- CHECK applies to the proposed row before ON CONFLICT, so never propose a
  -- negative count; a decrement always finds the existing row.
  INSERT INTO chart_stats(chart_id,upvotes,downvotes) VALUES(song,greatest(up,0),greatest(down,0))
  ON CONFLICT(chart_id) DO UPDATE SET upvotes=chart_stats.upvotes+up,downvotes=chart_stats.downvotes+down;
 END IF;
 RETURN NULL;
END;
$$;


--
-- Name: count_comment(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.count_comment() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
DECLARE delta integer; rec comments;
BEGIN
 IF TG_OP='INSERT' THEN delta:=1; rec:=NEW; ELSE delta:=-1; rec:=OLD; END IF;
 IF rec.parent_id IS NOT NULL THEN
  UPDATE comments SET reply_count=reply_count+delta WHERE id=rec.parent_id;
 END IF;
 IF rec.dan_id IS NOT NULL THEN
  UPDATE dan_courses SET comment_count=comment_count+delta WHERE id=rec.dan_id;
 ELSIF EXISTS(SELECT 1 FROM charts WHERE id=rec.chart_id) THEN
  INSERT INTO chart_stats(chart_id,comment_count) VALUES(rec.chart_id,greatest(delta,0))
  ON CONFLICT(chart_id) DO UPDATE SET comment_count=chart_stats.comment_count+delta;
 END IF;
 RETURN NULL;
END;
$$;


--
-- Name: count_comment_vote(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.count_comment_vote() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
DECLARE target text; up integer:=0; down integer:=0;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  target:=OLD.comment_id;
  IF OLD.value=1 THEN up:=up-1; ELSE down:=down-1; END IF;
 END IF;
 IF TG_OP IN ('INSERT','UPDATE') THEN
  target:=NEW.comment_id;
  IF NEW.value=1 THEN up:=up+1; ELSE down:=down+1; END IF;
 END IF;
 UPDATE comments SET upvotes=upvotes+up,downvotes=downvotes+down WHERE id=target;
 RETURN NULL;
END;
$$;


--
-- Name: count_dan_vote(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.count_dan_vote() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
DECLARE target text; up integer:=0; down integer:=0;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  target:=OLD.dan_id;
  IF OLD.value=1 THEN up:=up-1; ELSE down:=down-1; END IF;
 END IF;
 IF TG_OP IN ('INSERT','UPDATE') THEN
  target:=NEW.dan_id;
  IF NEW.value=1 THEN up:=up+1; ELSE down:=down+1; END IF;
 END IF;
 UPDATE dan_courses SET upvotes=upvotes+up,downvotes=downvotes+down WHERE id=target;
 RETURN NULL;
END;
$$;


--
-- Name: dan_attempt_scores(text); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.dan_attempt_scores(attempt text) RETURNS text[]
    LANGUAGE sql STABLE
    SET search_path TO 'official', 'net'
    AS $$
 SELECT COALESCE(array_agg(m.id ORDER BY g.position), '{}')
 FROM official.dan_attempts a
 CROSS JOIN LATERAL generate_series(0, cardinality(a.songs_played) - 1) g(position)
 LEFT JOIN LATERAL (
  SELECT sc.id FROM official.dan_course_songs s
  JOIN official.chart_difficulties d ON d.chart_id = s.chart_id AND d.course = s.course
  JOIN official.scores sc ON sc.difficulty_id = d.id AND sc.user_id = a.user_id
  WHERE s.course_id = a.course_id AND s.position = g.position
   AND sc.submitted_at <= a.submitted_at + interval '30 seconds'
   AND sc.submitted_at > GREATEST(a.submitted_at - interval '1 hour',
    COALESCE((SELECT max(b.submitted_at) FROM official.dan_attempts b WHERE b.user_id = a.user_id AND b.submitted_at < a.submitted_at) + interval '30 seconds', '-infinity'))
  ORDER BY sc.submitted_at DESC, sc.id LIMIT 1
 ) m ON true
 WHERE a.id = attempt
$$;


--
-- Name: protect_chart_owner(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.protect_chart_owner() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.id=OLD.id THEN RETURN NEW; END IF;
 IF EXISTS(SELECT 1 FROM charts WHERE owner_ids @> ARRAY[OLD.id]) THEN
  RAISE EXCEPTION 'user owns charts' USING ERRCODE='23503';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;


--
-- Name: retire_chart_files(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.retire_chart_files() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
 INSERT INTO retired_files(storage_key) SELECT storage_key FROM chart_resources
  WHERE id IN (OLD.tja_file,OLD.audio_file,OLD.cover_file,OLD.preview_file,OLD.movie_file,OLD.bgimage_file)
   AND (TG_OP='DELETE' OR NOT id=ANY(array_remove(ARRAY[NEW.tja_file,NEW.audio_file,NEW.cover_file,NEW.preview_file,NEW.movie_file,NEW.bgimage_file],NULL)))
  ON CONFLICT DO NOTHING;
 RETURN NULL;
END;
$$;


--
-- Name: retire_playlist_cover(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.retire_playlist_cover() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
 INSERT INTO retired_files(storage_key) SELECT storage_key FROM chart_resources
  WHERE id IN (OLD.cover_file,OLD.game_cover_file)
   AND (TG_OP='DELETE' OR NOT id=ANY(array_remove(ARRAY[NEW.cover_file,NEW.game_cover_file],NULL)))
  ON CONFLICT DO NOTHING;
 RETURN NULL;
END;
$$;


--
-- Name: touch_related_chart(); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.touch_related_chart() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
DECLARE target text;
BEGIN
  IF TG_OP = 'DELETE' THEN target := OLD.chart_id; ELSE target := NEW.chart_id; END IF;
  UPDATE official.charts SET update_at = greatest(update_at, clock_timestamp()) WHERE id = target;
  RETURN NULL;
END;
$$;


--
-- Name: valid_chart_owners(text[]); Type: FUNCTION; Schema: official; Owner: -
--

CREATE FUNCTION official.valid_chart_owners(ids text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    SET search_path TO 'official', 'net'
    AS $$
 SELECT COALESCE(array_ndims(ids)=1 AND array_lower(ids,1)=1 AND cardinality(ids)>0
  AND cardinality(ids)=(SELECT count(DISTINCT id) FROM unnest(ids) id WHERE id<>''),false)
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: chart_collaborations; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.chart_collaborations (
    id text NOT NULL,
    chart_id text NOT NULL,
    inviter_id text NOT NULL,
    recipient_id text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    responded_at timestamp with time zone,
    revoked_by text,
    CONSTRAINT chart_collaborations_check CHECK ((inviter_id <> recipient_id)),
    CONSTRAINT chart_collaborations_revoked_by CHECK (((status = 'revoked'::text) = (revoked_by IS NOT NULL))),
    CONSTRAINT chart_collaborations_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'declined'::text, 'revoked'::text])))
);


--
-- Name: chart_difficulties; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.chart_difficulties (
    id bigint NOT NULL,
    chart_id text NOT NULL,
    course text NOT NULL,
    side text NOT NULL,
    level smallint NOT NULL,
    branching boolean DEFAULT false NOT NULL,
    maker text DEFAULT ''::text NOT NULL,
    content_hash text,
    CONSTRAINT chart_difficulties_content_hash_check CHECK ((content_hash ~ '^v[0-9]+:[0-9a-f]{64}$'::text)),
    CONSTRAINT chart_difficulties_course_check CHECK ((course = ANY (ARRAY['Easy'::text, 'Normal'::text, 'Hard'::text, 'Oni'::text, 'Edit'::text]))),
    CONSTRAINT chart_difficulties_level_check CHECK (((level >= 1) AND (level <= 10))),
    CONSTRAINT chart_difficulties_maker_check CHECK ((octet_length(maker) <= 500)),
    CONSTRAINT chart_difficulties_side_check CHECK ((side = ANY (ARRAY['single'::text, 'p1'::text, 'p2'::text])))
);


--
-- Name: chart_difficulties_id_seq; Type: SEQUENCE; Schema: fanmade; Owner: -
--

ALTER TABLE fanmade.chart_difficulties ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME fanmade.chart_difficulties_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: chart_resources; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.chart_resources (
    id bigint NOT NULL,
    storage_key text NOT NULL,
    original_filename text NOT NULL,
    sha256 text NOT NULL,
    byte_size bigint NOT NULL,
    extension text NOT NULL,
    CONSTRAINT chart_resources_byte_size_check CHECK ((byte_size > 0)),
    CONSTRAINT chart_resources_extension_check CHECK ((extension ~ '^[a-z0-9]+$'::text)),
    CONSTRAINT chart_resources_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT chart_resources_storage_key_check CHECK ((storage_key <> ''::text))
);


--
-- Name: chart_resources_id_seq; Type: SEQUENCE; Schema: fanmade; Owner: -
--

ALTER TABLE fanmade.chart_resources ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME fanmade.chart_resources_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: chart_stats; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.chart_stats (
    chart_id text NOT NULL,
    upvotes integer DEFAULT 0 NOT NULL,
    downvotes integer DEFAULT 0 NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL,
    CONSTRAINT chart_stats_comment_count_check CHECK ((comment_count >= 0)),
    CONSTRAINT chart_stats_downvotes_check CHECK ((downvotes >= 0)),
    CONSTRAINT chart_stats_upvotes_check CHECK ((upvotes >= 0))
);


--
-- Name: chart_votes; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.chart_votes (
    chart_id text NOT NULL,
    user_id text NOT NULL,
    value smallint NOT NULL,
    voted_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chart_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: charts; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.charts (
    id text NOT NULL,
    owner_ids text[] CONSTRAINT charts_owner_id_not_null NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    title text NOT NULL,
    subtitle text DEFAULT ''::text NOT NULL,
    bpm double precision NOT NULL,
    offset_seconds double precision DEFAULT 0 NOT NULL,
    demo_start double precision DEFAULT 0 NOT NULL,
    duration double precision NOT NULL,
    title_translations jsonb DEFAULT '{}'::jsonb NOT NULL,
    subtitle_translations jsonb DEFAULT '{}'::jsonb NOT NULL,
    category_flags integer DEFAULT 16 NOT NULL,
    demo_end double precision NOT NULL,
    tja_file bigint NOT NULL,
    audio_file bigint NOT NULL,
    cover_file bigint,
    preview_file bigint,
    movie_file bigint,
    movie_offset double precision DEFAULT 0 NOT NULL,
    number_id bigint NOT NULL,
    update_at timestamp with time zone DEFAULT now() NOT NULL,
    bgimage_file bigint,
    nijiiro_id integer,
    CONSTRAINT charts_bpm_check CHECK (((bpm > (0)::double precision) AND (bpm < 'Infinity'::double precision))),
    CONSTRAINT charts_category_flags_check CHECK (((category_flags >= 0) AND ((category_flags & 63) = category_flags))),
    CONSTRAINT charts_duration_check CHECK (((duration > (0)::double precision) AND (duration <= (1200)::double precision))),
    CONSTRAINT charts_movie_offset_check CHECK (((movie_offset >= ('-1200'::integer)::double precision) AND (movie_offset <= (1200)::double precision))),
    CONSTRAINT charts_nijiiro_id_check CHECK ((nijiiro_id > 0)),
    CONSTRAINT charts_owners_valid CHECK (fanmade.valid_chart_owners(owner_ids)),
    CONSTRAINT charts_status_check CHECK ((status = ANY (ARRAY['published'::text, 'deleted'::text, 'hidden'::text]))),
    CONSTRAINT charts_subtitle_translations_check CHECK ((jsonb_typeof(subtitle_translations) = 'object'::text)),
    CONSTRAINT charts_title_translations_check CHECK ((jsonb_typeof(title_translations) = 'object'::text))
);


--
-- Name: charts_number_id_seq; Type: SEQUENCE; Schema: fanmade; Owner: -
--

ALTER TABLE fanmade.charts ALTER COLUMN number_id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME fanmade.charts_number_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: comment_votes; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.comment_votes (
    comment_id text NOT NULL,
    user_id text NOT NULL,
    value smallint NOT NULL,
    voted_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT comment_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: comments; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.comments (
    id text NOT NULL,
    chart_id text,
    parent_id text,
    root_id text NOT NULL,
    depth integer NOT NULL,
    author_id text NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    edited_at timestamp with time zone,
    deleted_at timestamp with time zone,
    removed_at timestamp with time zone,
    upvotes integer DEFAULT 0 NOT NULL,
    downvotes integer DEFAULT 0 NOT NULL,
    reply_count integer DEFAULT 0 NOT NULL,
    dan_id text,
    CONSTRAINT comments_body_check CHECK ((char_length(body) <= 10000)),
    CONSTRAINT comments_check CHECK ((((parent_id IS NULL) = (depth = 0)) AND ((parent_id IS NOT NULL) OR (root_id = id)))),
    CONSTRAINT comments_check1 CHECK ((((deleted_at IS NULL) AND (removed_at IS NULL)) = (body <> ''::text))),
    CONSTRAINT comments_check2 CHECK (((chart_id IS NULL) <> (dan_id IS NULL))),
    CONSTRAINT comments_depth_check CHECK (((depth >= 0) AND (depth <= 1000))),
    CONSTRAINT comments_downvotes_check CHECK ((downvotes >= 0)),
    CONSTRAINT comments_reply_count_check CHECK ((reply_count >= 0)),
    CONSTRAINT comments_upvotes_check CHECK ((upvotes >= 0))
);


--
-- Name: constants_calibrations; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.constants_calibrations (
    version integer NOT NULL,
    params jsonb NOT NULL,
    rating_version smallint NOT NULL,
    constants_version smallint NOT NULL,
    route_count integer NOT NULL,
    computed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT constants_calibrations_constants_version_check CHECK ((constants_version > 0)),
    CONSTRAINT constants_calibrations_rating_version_check CHECK ((rating_version > 0)),
    CONSTRAINT constants_calibrations_route_count_check CHECK ((route_count > 0)),
    CONSTRAINT constants_calibrations_version_check CHECK ((version > 0))
);


--
-- Name: dan_attempts; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.dan_attempts (
    id text NOT NULL,
    course_id text NOT NULL,
    user_id text NOT NULL,
    idempotency_key text NOT NULL,
    payload_digest text NOT NULL,
    verdict smallint NOT NULL,
    songs_played text[] NOT NULL,
    granted jsonb,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL,
    source text NOT NULL
)
PARTITION BY LIST (source);


--
-- Name: dan_attempts; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.dan_attempts (
    id text NOT NULL,
    course_id text NOT NULL,
    user_id text NOT NULL,
    idempotency_key text NOT NULL,
    payload_digest text NOT NULL,
    verdict smallint NOT NULL,
    songs_played text[] NOT NULL,
    granted jsonb,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL,
    source text DEFAULT 'fanmade'::text NOT NULL,
    CONSTRAINT dan_attempts_songs_played_check CHECK (((cardinality(songs_played) >= 0) AND (cardinality(songs_played) <= 3))),
    CONSTRAINT dan_attempts_source CHECK ((source = 'fanmade'::text)),
    CONSTRAINT dan_attempts_verdict_check CHECK (((verdict >= 0) AND (verdict <= 6)))
);


--
-- Name: dan_course_songs; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.dan_course_songs (
    course_id text NOT NULL,
    "position" smallint NOT NULL,
    chart_id text NOT NULL,
    course text NOT NULL,
    hidden boolean DEFAULT false NOT NULL,
    side text NOT NULL,
    valid boolean DEFAULT true NOT NULL,
    CONSTRAINT dan_course_songs_course_check CHECK ((course = ANY (ARRAY['Easy'::text, 'Normal'::text, 'Hard'::text, 'Oni'::text, 'Edit'::text]))),
    CONSTRAINT dan_course_songs_position_check CHECK ((("position" >= 0) AND ("position" <= 2))),
    CONSTRAINT dan_course_songs_side_check CHECK ((side = ANY (ARRAY['single'::text, 'p1'::text, 'p2'::text])))
);


--
-- Name: dan_courses; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.dan_courses (
    id text NOT NULL,
    owner_id text NOT NULL,
    title jsonb NOT NULL,
    color smallint DEFAULT 0 NOT NULL,
    exams jsonb NOT NULL,
    rewards jsonb DEFAULT '{}'::jsonb NOT NULL,
    official boolean DEFAULT false NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    upvotes integer DEFAULT 0 NOT NULL,
    downvotes integer DEFAULT 0 NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL,
    code text NOT NULL,
    CONSTRAINT dan_courses_code_check CHECK ((code ~ '^[A-Z0-9]{6}$'::text)),
    CONSTRAINT dan_courses_color_check CHECK (((color >= 0) AND (color <= 7))),
    CONSTRAINT dan_courses_comment_count_check CHECK ((comment_count >= 0)),
    CONSTRAINT dan_courses_downvotes_check CHECK ((downvotes >= 0)),
    CONSTRAINT dan_courses_exams_check CHECK ((jsonb_typeof(exams) = 'array'::text)),
    CONSTRAINT dan_courses_id_format CHECK ((id ~ '^[0-9a-f]{32}$'::text)),
    CONSTRAINT dan_courses_rewards_check CHECK ((jsonb_typeof(rewards) = 'object'::text)),
    CONSTRAINT dan_courses_status_check CHECK ((status = ANY (ARRAY['published'::text, 'deleted'::text]))),
    CONSTRAINT dan_courses_title_check CHECK (((jsonb_typeof(title) = 'object'::text) AND (title <> '{}'::jsonb))),
    CONSTRAINT dan_courses_upvotes_check CHECK ((upvotes >= 0))
);


--
-- Name: dan_votes; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.dan_votes (
    dan_id text NOT NULL,
    user_id text NOT NULL,
    value smallint NOT NULL,
    voted_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT dan_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: difficulty_constants; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.difficulty_constants (
    content_hash text NOT NULL,
    route text NOT NULL,
    constants_version smallint NOT NULL,
    note_count integer NOT NULL,
    raw_stamina double precision NOT NULL,
    raw_handspeed double precision NOT NULL,
    raw_burst double precision NOT NULL,
    raw_complex double precision NOT NULL,
    raw_complex_ratio double precision NOT NULL,
    raw_rhythm double precision NOT NULL,
    raw_rhythm_ratio double precision NOT NULL,
    calibration_version integer,
    sub_constant_1 double precision,
    main_constant double precision,
    sub_constant_2 double precision,
    stamina double precision,
    handspeed double precision,
    burst double precision,
    complex double precision,
    rhythm double precision,
    CONSTRAINT difficulty_constants_constants_version_check CHECK ((constants_version > 0)),
    CONSTRAINT difficulty_constants_content_hash_check CHECK ((content_hash ~ '^v[0-9]+:[0-9a-f]{64}$'::text)),
    CONSTRAINT difficulty_constants_note_count_check CHECK ((note_count >= 0)),
    CONSTRAINT difficulty_constants_rating_complete CHECK ((num_nulls(calibration_version, sub_constant_1, main_constant, sub_constant_2, stamina, handspeed, burst, complex, rhythm) = ANY (ARRAY[0, 9]))),
    CONSTRAINT difficulty_constants_route_check CHECK ((route = ANY (ARRAY['unbranched'::text, 'normal'::text, 'expert'::text, 'master'::text])))
);


--
-- Name: notification_mutes; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.notification_mutes (
    user_id text NOT NULL,
    kind text NOT NULL,
    CONSTRAINT notification_mutes_kind_check CHECK ((kind ~ '^[a-z][a-z_]{0,39}$'::text))
);


--
-- Name: notifications; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.notifications (
    id text NOT NULL,
    recipient_id text NOT NULL,
    kind text NOT NULL,
    actor_id text,
    chart_id text,
    comment_id text,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    dedupe_key text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    read_at timestamp with time zone,
    collaboration_id text,
    dan_id text,
    CONSTRAINT notifications_check CHECK (((actor_id IS NULL) OR (actor_id <> recipient_id))),
    CONSTRAINT notifications_data_check CHECK ((jsonb_typeof(data) = 'object'::text)),
    CONSTRAINT notifications_kind_check CHECK ((kind ~ '^[a-z][a-z_]{0,39}$'::text))
);


--
-- Name: pending_objects; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.pending_objects (
    storage_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: playlist_favorites; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.playlist_favorites (
    playlist_id text NOT NULL,
    user_id text NOT NULL,
    favorited_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: playlist_items; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.playlist_items (
    playlist_id text NOT NULL,
    chart_id text NOT NULL,
    "position" integer NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT playlist_items_position_check CHECK (("position" >= 0))
);


--
-- Name: playlists; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.playlists (
    id text NOT NULL,
    owner_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_public boolean DEFAULT false NOT NULL,
    category_flag integer DEFAULT 16 NOT NULL,
    cover_file bigint,
    game_cover_file bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT playlists_category_flag_check CHECK ((category_flag = ANY (ARRAY[1, 2, 4, 8, 16, 32]))),
    CONSTRAINT playlists_check CHECK (((cover_file IS NULL) = (game_cover_file IS NULL))),
    CONSTRAINT playlists_description_check CHECK ((char_length(description) <= 500)),
    CONSTRAINT playlists_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 50) AND (name = btrim(name))))
);


--
-- Name: retired_files; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.retired_files (
    storage_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: retired_score_requests; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.retired_score_requests (
    user_id text NOT NULL,
    idempotency_key text NOT NULL
);


--
-- Name: scores; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.scores (
    id text NOT NULL,
    user_id text NOT NULL,
    good integer NOT NULL,
    ok integer NOT NULL,
    bad integer NOT NULL,
    score bigint NOT NULL,
    drumroll integer NOT NULL,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL,
    idempotency_key text,
    payload_digest text NOT NULL,
    max_combo integer NOT NULL,
    replay_data jsonb,
    clear_status integer DEFAULT 0 NOT NULL,
    difficulty_id bigint NOT NULL,
    is_latest boolean DEFAULT true NOT NULL,
    adlib integer[] DEFAULT '{0,0}'::integer[] NOT NULL,
    mines integer DEFAULT 0 NOT NULL,
    source text NOT NULL
)
PARTITION BY LIST (source);


--
-- Name: scores; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.scores (
    id text NOT NULL,
    user_id text NOT NULL,
    good integer NOT NULL,
    ok integer NOT NULL,
    bad integer NOT NULL,
    score bigint NOT NULL,
    drumroll integer NOT NULL,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL,
    idempotency_key text,
    payload_digest text NOT NULL,
    max_combo integer NOT NULL,
    replay_data jsonb,
    clear_status integer DEFAULT 0 NOT NULL,
    difficulty_id bigint NOT NULL,
    is_latest boolean DEFAULT true NOT NULL,
    adlib integer[] DEFAULT '{0,0}'::integer[] NOT NULL,
    mines integer DEFAULT 0 NOT NULL,
    source text DEFAULT 'fanmade'::text NOT NULL,
    CONSTRAINT scores_adlib_check CHECK (((cardinality(adlib) = 2) AND (array_position(adlib, NULL::integer) IS NULL) AND (adlib[1] >= 0) AND (adlib[2] >= 0))),
    CONSTRAINT scores_bad_check CHECK ((bad >= 0)),
    CONSTRAINT scores_clear_status_check CHECK (((clear_status >= 0) AND (clear_status <= 3))),
    CONSTRAINT scores_drumroll_check CHECK ((drumroll >= 0)),
    CONSTRAINT scores_good_check CHECK ((good >= 0)),
    CONSTRAINT scores_max_combo_check CHECK ((max_combo >= 0)),
    CONSTRAINT scores_mines_check CHECK ((mines >= 0)),
    CONSTRAINT scores_ok_check CHECK ((ok >= 0)),
    CONSTRAINT scores_payload_digest_check CHECK ((length(payload_digest) = 64)),
    CONSTRAINT scores_replay_object CHECK (((replay_data IS NULL) OR (jsonb_typeof(replay_data) = 'object'::text))),
    CONSTRAINT scores_score_check CHECK (((score >= 0) AND (score <= '9007199254740991'::bigint))),
    CONSTRAINT scores_source CHECK ((source = 'fanmade'::text))
);


--
-- Name: upload_requests; Type: TABLE; Schema: fanmade; Owner: -
--

CREATE TABLE fanmade.upload_requests (
    user_id text NOT NULL,
    idempotency_key text NOT NULL,
    payload_digest text NOT NULL,
    chart_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    tja_sha256 text DEFAULT ''::text NOT NULL,
    audio_sha256 text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'done'::text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    CONSTRAINT upload_requests_status_check CHECK ((status = ANY (ARRAY['processing'::text, 'done'::text])))
);


--
-- Name: awards; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.awards (
    id bigint NOT NULL,
    service text NOT NULL,
    request_key text NOT NULL,
    user_id text NOT NULL,
    course_id text DEFAULT ''::text NOT NULL,
    course_title text DEFAULT ''::text NOT NULL,
    official boolean DEFAULT false NOT NULL,
    verdict smallint DEFAULT 0 NOT NULL,
    dan smallint,
    granted jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT awards_dan_check CHECK (((dan >= 0) AND (dan <= 24))),
    CONSTRAINT awards_request_key_check CHECK (((length(request_key) >= 1) AND (length(request_key) <= 100))),
    CONSTRAINT awards_verdict_check CHECK (((verdict >= 0) AND (verdict <= 6)))
);


--
-- Name: awards_id_seq; Type: SEQUENCE; Schema: net; Owner: -
--

ALTER TABLE net.awards ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME net.awards_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: group_changes; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.group_changes (
    id bigint NOT NULL,
    user_id text NOT NULL,
    groups_before integer NOT NULL,
    groups_after integer NOT NULL,
    changed_by text NOT NULL,
    changed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: group_changes_id_seq; Type: SEQUENCE; Schema: net; Owner: -
--

ALTER TABLE net.group_changes ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME net.group_changes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: nameplates; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.nameplates (
    user_id text NOT NULL,
    title_id text,
    show_dan boolean DEFAULT true NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: oidc_flows; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.oidc_flows (
    state_hash text NOT NULL,
    binding_hash text NOT NULL,
    nonce text NOT NULL,
    verifier text NOT NULL,
    return_path text NOT NULL,
    expires_at timestamp with time zone NOT NULL
);


--
-- Name: schema_migrations; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.schema_migrations (
    version integer NOT NULL,
    applied_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: services; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.services (
    id text NOT NULL,
    name text NOT NULL,
    key_hash text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT services_id_check CHECK ((id ~ '^[a-z0-9][a-z0-9-]{0,39}$'::text)),
    CONSTRAINT services_name_check CHECK (((length(name) >= 1) AND (length(name) <= 80)))
);


--
-- Name: sessions; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.sessions (
    token_hash text NOT NULL,
    user_id text NOT NULL,
    csrf_token text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    upstream_token bytea NOT NULL,
    upstream_refresh_token bytea,
    upstream_expires_at timestamp with time zone
);


--
-- Name: titles; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.titles (
    id text NOT NULL,
    name text NOT NULL,
    name_translations jsonb DEFAULT '{}'::jsonb NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    style text NOT NULL,
    official boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    owner_id text,
    CONSTRAINT titles_description_check CHECK ((length(description) <= 500)),
    CONSTRAINT titles_id_check CHECK ((id ~ '^[a-z0-9][a-z0-9-]{0,63}$'::text)),
    CONSTRAINT titles_name_check CHECK (((length(name) >= 1) AND (length(name) <= 40))),
    CONSTRAINT titles_name_translations_check CHECK ((jsonb_typeof(name_translations) = 'object'::text)),
    CONSTRAINT titles_style_check CHECK ((style = ANY (ARRAY['wood'::text, 'teal'::text, 'wood2'::text, 'purple'::text, 'gold'::text, 'rainbow'::text])))
);


--
-- Name: user_dans; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.user_dans (
    user_id text NOT NULL,
    dan smallint NOT NULL,
    verdict smallint NOT NULL,
    award_id bigint,
    achieved_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_dans_dan_check CHECK (((dan >= 0) AND (dan <= 24))),
    CONSTRAINT user_dans_verdict_check CHECK (((verdict >= 1) AND (verdict <= 6)))
);


--
-- Name: user_titles; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.user_titles (
    user_id text NOT NULL,
    title_id text NOT NULL,
    award_id bigint,
    granted_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: net; Owner: -
--

CREATE TABLE net.users (
    id text NOT NULL,
    first_login_at timestamp with time zone,
    last_active_at timestamp with time zone,
    groups integer DEFAULT 0 NOT NULL,
    CONSTRAINT users_activity_order CHECK (((first_login_at IS NULL) OR ((last_active_at IS NOT NULL) AND (last_active_at >= first_login_at)))),
    CONSTRAINT users_groups CHECK ((groups >= 0))
);


--
-- Name: chart_collaborations; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.chart_collaborations (
    id text NOT NULL,
    chart_id text NOT NULL,
    inviter_id text NOT NULL,
    recipient_id text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    responded_at timestamp with time zone,
    revoked_by text,
    CONSTRAINT chart_collaborations_check CHECK ((inviter_id <> recipient_id)),
    CONSTRAINT chart_collaborations_revoked_by CHECK (((status = 'revoked'::text) = (revoked_by IS NOT NULL))),
    CONSTRAINT chart_collaborations_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'declined'::text, 'revoked'::text])))
);


--
-- Name: chart_difficulties; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.chart_difficulties (
    id bigint NOT NULL,
    chart_id text NOT NULL,
    course text NOT NULL,
    side text NOT NULL,
    level smallint NOT NULL,
    branching boolean DEFAULT false NOT NULL,
    maker text DEFAULT ''::text NOT NULL,
    content_hash text,
    CONSTRAINT chart_difficulties_content_hash_check CHECK ((content_hash ~ '^v[0-9]+:[0-9a-f]{64}$'::text)),
    CONSTRAINT chart_difficulties_course_check CHECK ((course = ANY (ARRAY['Easy'::text, 'Normal'::text, 'Hard'::text, 'Oni'::text, 'Edit'::text]))),
    CONSTRAINT chart_difficulties_level_check CHECK (((level >= 1) AND (level <= 10))),
    CONSTRAINT chart_difficulties_maker_check CHECK ((octet_length(maker) <= 500)),
    CONSTRAINT chart_difficulties_side_check CHECK ((side = ANY (ARRAY['single'::text, 'p1'::text, 'p2'::text])))
);


--
-- Name: chart_difficulties_id_seq; Type: SEQUENCE; Schema: official; Owner: -
--

ALTER TABLE official.chart_difficulties ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME official.chart_difficulties_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: chart_resources; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.chart_resources (
    id bigint NOT NULL,
    storage_key text NOT NULL,
    original_filename text NOT NULL,
    sha256 text NOT NULL,
    byte_size bigint NOT NULL,
    extension text NOT NULL,
    CONSTRAINT chart_resources_byte_size_check CHECK ((byte_size > 0)),
    CONSTRAINT chart_resources_check CHECK ((storage_key = ((sha256 || '.'::text) || extension))),
    CONSTRAINT chart_resources_extension_check CHECK ((extension ~ '^[a-z0-9]+$'::text)),
    CONSTRAINT chart_resources_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text))
);


--
-- Name: chart_resources_id_seq; Type: SEQUENCE; Schema: official; Owner: -
--

ALTER TABLE official.chart_resources ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME official.chart_resources_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: chart_stats; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.chart_stats (
    chart_id text NOT NULL,
    upvotes integer DEFAULT 0 NOT NULL,
    downvotes integer DEFAULT 0 NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL,
    CONSTRAINT chart_stats_comment_count_check CHECK ((comment_count >= 0)),
    CONSTRAINT chart_stats_downvotes_check CHECK ((downvotes >= 0)),
    CONSTRAINT chart_stats_upvotes_check CHECK ((upvotes >= 0))
);


--
-- Name: chart_votes; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.chart_votes (
    chart_id text NOT NULL,
    user_id text NOT NULL,
    value smallint NOT NULL,
    voted_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chart_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: charts; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.charts (
    id text NOT NULL,
    nijiiro_id integer,
    official_id text,
    owner_ids text[] CONSTRAINT charts_uploader_id_not_null NOT NULL,
    tja_file bigint,
    audio_file bigint,
    cover_file bigint,
    preview_file bigint,
    description text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    update_at timestamp with time zone DEFAULT now() NOT NULL,
    title text NOT NULL,
    subtitle text DEFAULT ''::text NOT NULL,
    bpm double precision NOT NULL,
    offset_seconds double precision DEFAULT 0 NOT NULL,
    demo_start double precision DEFAULT 0 NOT NULL,
    duration double precision NOT NULL,
    title_translations jsonb DEFAULT '{}'::jsonb NOT NULL,
    subtitle_translations jsonb DEFAULT '{}'::jsonb NOT NULL,
    category_flags smallint DEFAULT 0 NOT NULL,
    demo_end double precision NOT NULL,
    movie_file bigint,
    movie_offset double precision DEFAULT 0 NOT NULL,
    number_id bigint NOT NULL,
    bgimage_file bigint,
    CONSTRAINT charts_bpm_check CHECK (((bpm >= (0)::double precision) AND (bpm < 'Infinity'::double precision) AND ((tja_file IS NULL) OR (bpm > (0)::double precision)))),
    CONSTRAINT charts_category_flags_check CHECK (((category_flags >= 0) AND (category_flags <= 255))),
    CONSTRAINT charts_duration_check CHECK (((duration >= (0)::double precision) AND (duration <= (1200)::double precision) AND ((tja_file IS NULL) OR (duration > (0)::double precision)))),
    CONSTRAINT charts_files_check CHECK (((status = 'deleted'::text) OR ((tja_file IS NULL) = (audio_file IS NULL)))),
    CONSTRAINT charts_id_format CHECK ((id ~ '^[0-9a-f]{32}$'::text)),
    CONSTRAINT charts_movie_offset_check CHECK (((movie_offset >= ('-1200'::integer)::double precision) AND (movie_offset <= (1200)::double precision))),
    CONSTRAINT charts_owners_valid CHECK (official.valid_chart_owners(owner_ids)),
    CONSTRAINT charts_status_check CHECK ((status = ANY (ARRAY['published'::text, 'deleted'::text, 'hidden'::text]))),
    CONSTRAINT charts_subtitle_translations_check CHECK ((jsonb_typeof(subtitle_translations) = 'object'::text)),
    CONSTRAINT charts_title_translations_check CHECK ((jsonb_typeof(title_translations) = 'object'::text))
);


--
-- Name: charts_number_id_seq; Type: SEQUENCE; Schema: official; Owner: -
--

ALTER TABLE official.charts ALTER COLUMN number_id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME official.charts_number_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: comment_votes; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.comment_votes (
    comment_id text NOT NULL,
    user_id text NOT NULL,
    value smallint NOT NULL,
    voted_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT comment_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: comments; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.comments (
    id text NOT NULL,
    chart_id text,
    parent_id text,
    root_id text NOT NULL,
    depth integer NOT NULL,
    author_id text NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    edited_at timestamp with time zone,
    deleted_at timestamp with time zone,
    removed_at timestamp with time zone,
    upvotes integer DEFAULT 0 NOT NULL,
    downvotes integer DEFAULT 0 NOT NULL,
    reply_count integer DEFAULT 0 NOT NULL,
    dan_id text,
    CONSTRAINT comments_body_check CHECK ((char_length(body) <= 10000)),
    CONSTRAINT comments_check CHECK ((((parent_id IS NULL) = (depth = 0)) AND ((parent_id IS NOT NULL) OR (root_id = id)))),
    CONSTRAINT comments_check1 CHECK ((((deleted_at IS NULL) AND (removed_at IS NULL)) = (body <> ''::text))),
    CONSTRAINT comments_check2 CHECK (((chart_id IS NULL) <> (dan_id IS NULL))),
    CONSTRAINT comments_depth_check CHECK (((depth >= 0) AND (depth <= 1000))),
    CONSTRAINT comments_downvotes_check CHECK ((downvotes >= 0)),
    CONSTRAINT comments_reply_count_check CHECK ((reply_count >= 0)),
    CONSTRAINT comments_upvotes_check CHECK ((upvotes >= 0))
);


--
-- Name: constants_calibrations; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.constants_calibrations (
    version integer NOT NULL,
    params jsonb NOT NULL,
    rating_version smallint NOT NULL,
    constants_version smallint NOT NULL,
    route_count integer NOT NULL,
    computed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT constants_calibrations_constants_version_check CHECK ((constants_version > 0)),
    CONSTRAINT constants_calibrations_rating_version_check CHECK ((rating_version > 0)),
    CONSTRAINT constants_calibrations_route_count_check CHECK ((route_count > 0)),
    CONSTRAINT constants_calibrations_version_check CHECK ((version > 0))
);


--
-- Name: dan_attempts; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.dan_attempts (
    id text NOT NULL,
    course_id text NOT NULL,
    user_id text NOT NULL,
    idempotency_key text NOT NULL,
    payload_digest text NOT NULL,
    verdict smallint NOT NULL,
    songs_played text[] NOT NULL,
    granted jsonb,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL,
    source text DEFAULT 'official'::text NOT NULL,
    CONSTRAINT dan_attempts_songs_played_check CHECK (((cardinality(songs_played) >= 0) AND (cardinality(songs_played) <= 3))),
    CONSTRAINT dan_attempts_source CHECK ((source = 'official'::text)),
    CONSTRAINT dan_attempts_verdict_check CHECK (((verdict >= 0) AND (verdict <= 6)))
);


--
-- Name: dan_course_songs; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.dan_course_songs (
    course_id text NOT NULL,
    "position" smallint NOT NULL,
    chart_id text NOT NULL,
    course text NOT NULL,
    hidden boolean DEFAULT false NOT NULL,
    side text NOT NULL,
    valid boolean DEFAULT true NOT NULL,
    CONSTRAINT dan_course_songs_course_check CHECK ((course = ANY (ARRAY['Easy'::text, 'Normal'::text, 'Hard'::text, 'Oni'::text, 'Edit'::text]))),
    CONSTRAINT dan_course_songs_position_check CHECK ((("position" >= 0) AND ("position" <= 2))),
    CONSTRAINT dan_course_songs_side_check CHECK ((side = ANY (ARRAY['single'::text, 'p1'::text, 'p2'::text])))
);


--
-- Name: dan_courses; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.dan_courses (
    id text NOT NULL,
    owner_id text NOT NULL,
    title jsonb NOT NULL,
    color smallint DEFAULT 0 NOT NULL,
    exams jsonb NOT NULL,
    rewards jsonb DEFAULT '{}'::jsonb NOT NULL,
    official boolean DEFAULT false NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    code text NOT NULL,
    upvotes integer DEFAULT 0 NOT NULL,
    downvotes integer DEFAULT 0 NOT NULL,
    comment_count integer DEFAULT 0 NOT NULL,
    CONSTRAINT dan_courses_code_check CHECK ((code ~ '^[A-Z0-9]{6}$'::text)),
    CONSTRAINT dan_courses_color_check CHECK (((color >= 0) AND (color <= 7))),
    CONSTRAINT dan_courses_comment_count_check CHECK ((comment_count >= 0)),
    CONSTRAINT dan_courses_downvotes_check CHECK ((downvotes >= 0)),
    CONSTRAINT dan_courses_exams_check CHECK ((jsonb_typeof(exams) = 'array'::text)),
    CONSTRAINT dan_courses_id_format CHECK ((id ~ '^[0-9a-f]{32}$'::text)),
    CONSTRAINT dan_courses_rewards_check CHECK ((jsonb_typeof(rewards) = 'object'::text)),
    CONSTRAINT dan_courses_status_check CHECK ((status = ANY (ARRAY['published'::text, 'deleted'::text]))),
    CONSTRAINT dan_courses_title_check CHECK (((jsonb_typeof(title) = 'object'::text) AND (title <> '{}'::jsonb))),
    CONSTRAINT dan_courses_upvotes_check CHECK ((upvotes >= 0))
);


--
-- Name: dan_votes; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.dan_votes (
    dan_id text NOT NULL,
    user_id text NOT NULL,
    value smallint NOT NULL,
    voted_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT dan_votes_value_check CHECK ((value = ANY (ARRAY['-1'::integer, 1])))
);


--
-- Name: difficulty_constants; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.difficulty_constants (
    content_hash text NOT NULL,
    route text NOT NULL,
    constants_version smallint NOT NULL,
    note_count integer NOT NULL,
    raw_stamina double precision NOT NULL,
    raw_handspeed double precision NOT NULL,
    raw_burst double precision NOT NULL,
    raw_complex double precision NOT NULL,
    raw_complex_ratio double precision NOT NULL,
    raw_rhythm double precision NOT NULL,
    raw_rhythm_ratio double precision NOT NULL,
    calibration_version integer,
    sub_constant_1 double precision,
    main_constant double precision,
    sub_constant_2 double precision,
    stamina double precision,
    handspeed double precision,
    burst double precision,
    complex double precision,
    rhythm double precision,
    CONSTRAINT difficulty_constants_constants_version_check CHECK ((constants_version > 0)),
    CONSTRAINT difficulty_constants_content_hash_check CHECK ((content_hash ~ '^v[0-9]+:[0-9a-f]{64}$'::text)),
    CONSTRAINT difficulty_constants_note_count_check CHECK ((note_count >= 0)),
    CONSTRAINT difficulty_constants_rating_complete CHECK ((num_nulls(calibration_version, sub_constant_1, main_constant, sub_constant_2, stamina, handspeed, burst, complex, rhythm) = ANY (ARRAY[0, 9]))),
    CONSTRAINT difficulty_constants_route_check CHECK ((route = ANY (ARRAY['unbranched'::text, 'normal'::text, 'expert'::text, 'master'::text])))
);


--
-- Name: notification_mutes; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.notification_mutes (
    user_id text NOT NULL,
    kind text NOT NULL,
    CONSTRAINT notification_mutes_kind_check CHECK ((kind ~ '^[a-z][a-z_]{0,39}$'::text))
);


--
-- Name: notifications; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.notifications (
    id text NOT NULL,
    recipient_id text NOT NULL,
    kind text NOT NULL,
    actor_id text,
    chart_id text,
    comment_id text,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    dedupe_key text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    read_at timestamp with time zone,
    dan_id text,
    collaboration_id text,
    CONSTRAINT notifications_check CHECK (((actor_id IS NULL) OR (actor_id <> recipient_id))),
    CONSTRAINT notifications_data_check CHECK ((jsonb_typeof(data) = 'object'::text)),
    CONSTRAINT notifications_kind_check CHECK ((kind ~ '^[a-z][a-z_]{0,39}$'::text))
);


--
-- Name: pending_objects; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.pending_objects (
    storage_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: playlist_favorites; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.playlist_favorites (
    playlist_id text NOT NULL,
    user_id text NOT NULL,
    favorited_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: playlist_items; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.playlist_items (
    playlist_id text NOT NULL,
    chart_id text NOT NULL,
    "position" integer NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT playlist_items_position_check CHECK (("position" >= 0))
);


--
-- Name: playlists; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.playlists (
    id text NOT NULL,
    owner_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_public boolean DEFAULT false NOT NULL,
    category_flag smallint DEFAULT 32 NOT NULL,
    cover_file bigint,
    game_cover_file bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT playlists_category_flag_check CHECK ((category_flag = ANY (ARRAY[1, 2, 4, 8, 16, 32, 64, 128]))),
    CONSTRAINT playlists_check CHECK (((cover_file IS NULL) = (game_cover_file IS NULL))),
    CONSTRAINT playlists_description_check CHECK ((char_length(description) <= 500)),
    CONSTRAINT playlists_id_format CHECK ((id ~ '^[0-9a-f]{32}$'::text)),
    CONSTRAINT playlists_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 50) AND (name = btrim(name))))
);


--
-- Name: retired_files; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.retired_files (
    storage_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: retired_score_requests; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.retired_score_requests (
    user_id text NOT NULL,
    idempotency_key text NOT NULL
);


--
-- Name: scores; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.scores (
    id text NOT NULL,
    user_id text NOT NULL,
    good integer NOT NULL,
    ok integer NOT NULL,
    bad integer NOT NULL,
    score bigint NOT NULL,
    drumroll integer NOT NULL,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL,
    idempotency_key text,
    payload_digest text NOT NULL,
    max_combo integer NOT NULL,
    replay_data jsonb,
    clear_status integer DEFAULT 0 NOT NULL,
    difficulty_id bigint NOT NULL,
    is_latest boolean DEFAULT true NOT NULL,
    adlib integer[] DEFAULT '{0,0}'::integer[] NOT NULL,
    mines integer DEFAULT 0 NOT NULL,
    source text DEFAULT 'official'::text NOT NULL,
    CONSTRAINT scores_adlib_check CHECK (((cardinality(adlib) = 2) AND (array_position(adlib, NULL::integer) IS NULL) AND (adlib[1] >= 0) AND (adlib[2] >= 0))),
    CONSTRAINT scores_bad_check CHECK ((bad >= 0)),
    CONSTRAINT scores_clear_status_check CHECK (((clear_status >= 0) AND (clear_status <= 3))),
    CONSTRAINT scores_drumroll_check CHECK ((drumroll >= 0)),
    CONSTRAINT scores_good_check CHECK ((good >= 0)),
    CONSTRAINT scores_max_combo_check CHECK ((max_combo >= 0)),
    CONSTRAINT scores_mines_check CHECK ((mines >= 0)),
    CONSTRAINT scores_ok_check CHECK ((ok >= 0)),
    CONSTRAINT scores_payload_digest_check CHECK ((length(payload_digest) = 64)),
    CONSTRAINT scores_replay_data_check CHECK (((replay_data IS NULL) OR (jsonb_typeof(replay_data) = 'object'::text))),
    CONSTRAINT scores_score_check CHECK (((score >= 0) AND (score <= '9007199254740991'::bigint))),
    CONSTRAINT scores_source CHECK ((source = 'official'::text))
);


--
-- Name: upload_requests; Type: TABLE; Schema: official; Owner: -
--

CREATE TABLE official.upload_requests (
    user_id text NOT NULL,
    idempotency_key text NOT NULL,
    payload_digest text NOT NULL,
    chart_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    tja_sha256 text DEFAULT ''::text NOT NULL,
    audio_sha256 text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'done'::text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    CONSTRAINT upload_requests_status_check CHECK ((status = ANY (ARRAY['processing'::text, 'done'::text])))
);


--
-- Name: dan_attempts; Type: TABLE ATTACH; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY net.dan_attempts ATTACH PARTITION fanmade.dan_attempts FOR VALUES IN ('fanmade');


--
-- Name: scores; Type: TABLE ATTACH; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY net.scores ATTACH PARTITION fanmade.scores FOR VALUES IN ('fanmade');


--
-- Name: dan_attempts; Type: TABLE ATTACH; Schema: official; Owner: -
--

ALTER TABLE ONLY net.dan_attempts ATTACH PARTITION official.dan_attempts FOR VALUES IN ('official');


--
-- Name: scores; Type: TABLE ATTACH; Schema: official; Owner: -
--

ALTER TABLE ONLY net.scores ATTACH PARTITION official.scores FOR VALUES IN ('official');


--
-- Name: chart_collaborations chart_collaborations_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_collaborations
    ADD CONSTRAINT chart_collaborations_pkey PRIMARY KEY (id);


--
-- Name: chart_difficulties chart_difficulties_chart_id_course_side_key; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_difficulties
    ADD CONSTRAINT chart_difficulties_chart_id_course_side_key UNIQUE (chart_id, course, side);


--
-- Name: chart_difficulties chart_difficulties_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_difficulties
    ADD CONSTRAINT chart_difficulties_pkey PRIMARY KEY (id);


--
-- Name: chart_resources chart_resources_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_resources
    ADD CONSTRAINT chart_resources_pkey PRIMARY KEY (id);


--
-- Name: chart_resources chart_resources_storage_key_key; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_resources
    ADD CONSTRAINT chart_resources_storage_key_key UNIQUE (storage_key);


--
-- Name: chart_stats chart_stats_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_stats
    ADD CONSTRAINT chart_stats_pkey PRIMARY KEY (chart_id);


--
-- Name: chart_votes chart_votes_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_votes
    ADD CONSTRAINT chart_votes_pkey PRIMARY KEY (chart_id, user_id);


--
-- Name: charts charts_number_id_key; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_number_id_key UNIQUE (number_id);


--
-- Name: charts charts_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_pkey PRIMARY KEY (id);


--
-- Name: comment_votes comment_votes_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comment_votes
    ADD CONSTRAINT comment_votes_pkey PRIMARY KEY (comment_id, user_id);


--
-- Name: comments comments_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comments
    ADD CONSTRAINT comments_pkey PRIMARY KEY (id);


--
-- Name: constants_calibrations constants_calibrations_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.constants_calibrations
    ADD CONSTRAINT constants_calibrations_pkey PRIMARY KEY (version);


--
-- Name: dan_attempts dan_attempts_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_attempts
    ADD CONSTRAINT dan_attempts_pkey PRIMARY KEY (id);


--
-- Name: dan_attempts dan_attempts_source_id_key; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.dan_attempts
    ADD CONSTRAINT dan_attempts_source_id_key UNIQUE (source, id);


--
-- Name: dan_attempts dan_attempts_source_id_key; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_attempts
    ADD CONSTRAINT dan_attempts_source_id_key UNIQUE (source, id);


--
-- Name: dan_attempts dan_attempts_user_id_idempotency_key_key; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_attempts
    ADD CONSTRAINT dan_attempts_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key);


--
-- Name: dan_course_songs dan_course_songs_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_course_songs
    ADD CONSTRAINT dan_course_songs_pkey PRIMARY KEY (course_id, "position");


--
-- Name: dan_courses dan_courses_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_courses
    ADD CONSTRAINT dan_courses_pkey PRIMARY KEY (id);


--
-- Name: dan_votes dan_votes_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_votes
    ADD CONSTRAINT dan_votes_pkey PRIMARY KEY (dan_id, user_id);


--
-- Name: difficulty_constants difficulty_constants_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.difficulty_constants
    ADD CONSTRAINT difficulty_constants_pkey PRIMARY KEY (content_hash, route);


--
-- Name: notification_mutes notification_mutes_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notification_mutes
    ADD CONSTRAINT notification_mutes_pkey PRIMARY KEY (user_id, kind);


--
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);


--
-- Name: pending_objects pending_objects_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.pending_objects
    ADD CONSTRAINT pending_objects_pkey PRIMARY KEY (storage_key);


--
-- Name: playlist_favorites playlist_favorites_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlist_favorites
    ADD CONSTRAINT playlist_favorites_pkey PRIMARY KEY (playlist_id, user_id);


--
-- Name: playlist_items playlist_items_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlist_items
    ADD CONSTRAINT playlist_items_pkey PRIMARY KEY (playlist_id, chart_id);


--
-- Name: playlists playlists_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlists
    ADD CONSTRAINT playlists_pkey PRIMARY KEY (id);


--
-- Name: retired_files retired_files_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.retired_files
    ADD CONSTRAINT retired_files_pkey PRIMARY KEY (storage_key);


--
-- Name: retired_score_requests retired_score_requests_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.retired_score_requests
    ADD CONSTRAINT retired_score_requests_pkey PRIMARY KEY (user_id, idempotency_key);


--
-- Name: scores scores_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.scores
    ADD CONSTRAINT scores_pkey PRIMARY KEY (id);


--
-- Name: scores scores_source_id_key; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.scores
    ADD CONSTRAINT scores_source_id_key UNIQUE (source, id);


--
-- Name: scores scores_source_id_key; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.scores
    ADD CONSTRAINT scores_source_id_key UNIQUE (source, id);


--
-- Name: scores scores_user_id_idempotency_key_key; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.scores
    ADD CONSTRAINT scores_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key);


--
-- Name: upload_requests upload_requests_pkey; Type: CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.upload_requests
    ADD CONSTRAINT upload_requests_pkey PRIMARY KEY (user_id, idempotency_key);


--
-- Name: awards awards_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.awards
    ADD CONSTRAINT awards_pkey PRIMARY KEY (id);


--
-- Name: awards awards_service_request_key_key; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.awards
    ADD CONSTRAINT awards_service_request_key_key UNIQUE (service, request_key);


--
-- Name: group_changes group_changes_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.group_changes
    ADD CONSTRAINT group_changes_pkey PRIMARY KEY (id);


--
-- Name: nameplates nameplates_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.nameplates
    ADD CONSTRAINT nameplates_pkey PRIMARY KEY (user_id);


--
-- Name: oidc_flows oidc_flows_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.oidc_flows
    ADD CONSTRAINT oidc_flows_pkey PRIMARY KEY (state_hash);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: services services_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.services
    ADD CONSTRAINT services_pkey PRIMARY KEY (id);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (token_hash);


--
-- Name: titles titles_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.titles
    ADD CONSTRAINT titles_pkey PRIMARY KEY (id);


--
-- Name: user_dans user_dans_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.user_dans
    ADD CONSTRAINT user_dans_pkey PRIMARY KEY (user_id, dan);


--
-- Name: user_titles user_titles_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.user_titles
    ADD CONSTRAINT user_titles_pkey PRIMARY KEY (user_id, title_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: chart_collaborations chart_collaborations_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_collaborations
    ADD CONSTRAINT chart_collaborations_pkey PRIMARY KEY (id);


--
-- Name: chart_difficulties chart_difficulties_chart_id_course_side_key; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_difficulties
    ADD CONSTRAINT chart_difficulties_chart_id_course_side_key UNIQUE (chart_id, course, side);


--
-- Name: chart_difficulties chart_difficulties_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_difficulties
    ADD CONSTRAINT chart_difficulties_pkey PRIMARY KEY (id);


--
-- Name: chart_resources chart_resources_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_resources
    ADD CONSTRAINT chart_resources_pkey PRIMARY KEY (id);


--
-- Name: chart_resources chart_resources_storage_key_key; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_resources
    ADD CONSTRAINT chart_resources_storage_key_key UNIQUE (storage_key);


--
-- Name: chart_stats chart_stats_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_stats
    ADD CONSTRAINT chart_stats_pkey PRIMARY KEY (chart_id);


--
-- Name: chart_votes chart_votes_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_votes
    ADD CONSTRAINT chart_votes_pkey PRIMARY KEY (chart_id, user_id);


--
-- Name: charts charts_number_id_key; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_number_id_key UNIQUE (number_id);


--
-- Name: charts charts_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_pkey PRIMARY KEY (id);


--
-- Name: comment_votes comment_votes_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comment_votes
    ADD CONSTRAINT comment_votes_pkey PRIMARY KEY (comment_id, user_id);


--
-- Name: comments comments_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comments
    ADD CONSTRAINT comments_pkey PRIMARY KEY (id);


--
-- Name: constants_calibrations constants_calibrations_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.constants_calibrations
    ADD CONSTRAINT constants_calibrations_pkey PRIMARY KEY (version);


--
-- Name: dan_attempts dan_attempts_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_attempts
    ADD CONSTRAINT dan_attempts_pkey PRIMARY KEY (id);


--
-- Name: dan_attempts dan_attempts_source_id_key; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_attempts
    ADD CONSTRAINT dan_attempts_source_id_key UNIQUE (source, id);


--
-- Name: dan_attempts dan_attempts_user_id_idempotency_key_key; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_attempts
    ADD CONSTRAINT dan_attempts_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key);


--
-- Name: dan_course_songs dan_course_songs_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_course_songs
    ADD CONSTRAINT dan_course_songs_pkey PRIMARY KEY (course_id, "position");


--
-- Name: dan_courses dan_courses_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_courses
    ADD CONSTRAINT dan_courses_pkey PRIMARY KEY (id);


--
-- Name: dan_votes dan_votes_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_votes
    ADD CONSTRAINT dan_votes_pkey PRIMARY KEY (dan_id, user_id);


--
-- Name: difficulty_constants difficulty_constants_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.difficulty_constants
    ADD CONSTRAINT difficulty_constants_pkey PRIMARY KEY (content_hash, route);


--
-- Name: notification_mutes notification_mutes_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notification_mutes
    ADD CONSTRAINT notification_mutes_pkey PRIMARY KEY (user_id, kind);


--
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);


--
-- Name: pending_objects pending_objects_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.pending_objects
    ADD CONSTRAINT pending_objects_pkey PRIMARY KEY (storage_key);


--
-- Name: playlist_favorites playlist_favorites_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlist_favorites
    ADD CONSTRAINT playlist_favorites_pkey PRIMARY KEY (playlist_id, user_id);


--
-- Name: playlist_items playlist_items_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlist_items
    ADD CONSTRAINT playlist_items_pkey PRIMARY KEY (playlist_id, chart_id);


--
-- Name: playlists playlists_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlists
    ADD CONSTRAINT playlists_pkey PRIMARY KEY (id);


--
-- Name: retired_files retired_files_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.retired_files
    ADD CONSTRAINT retired_files_pkey PRIMARY KEY (storage_key);


--
-- Name: retired_score_requests retired_score_requests_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.retired_score_requests
    ADD CONSTRAINT retired_score_requests_pkey PRIMARY KEY (user_id, idempotency_key);


--
-- Name: scores scores_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.scores
    ADD CONSTRAINT scores_pkey PRIMARY KEY (id);


--
-- Name: scores scores_source_id_key; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.scores
    ADD CONSTRAINT scores_source_id_key UNIQUE (source, id);


--
-- Name: scores scores_user_id_idempotency_key_key; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.scores
    ADD CONSTRAINT scores_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key);


--
-- Name: upload_requests upload_requests_pkey; Type: CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.upload_requests
    ADD CONSTRAINT upload_requests_pkey PRIMARY KEY (user_id, idempotency_key);


--
-- Name: chart_collaborations_inviter; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX chart_collaborations_inviter ON fanmade.chart_collaborations USING btree (inviter_id, created_at);


--
-- Name: chart_collaborations_pending; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE UNIQUE INDEX chart_collaborations_pending ON fanmade.chart_collaborations USING btree (chart_id, recipient_id) WHERE (status = 'pending'::text);


--
-- Name: chart_difficulties_content_hash; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX chart_difficulties_content_hash ON fanmade.chart_difficulties USING btree (content_hash);


--
-- Name: chart_difficulties_course_level; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX chart_difficulties_course_level ON fanmade.chart_difficulties USING btree (course, level, chart_id);


--
-- Name: chart_stats_score; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX chart_stats_score ON fanmade.chart_stats USING btree (((upvotes - downvotes)) DESC);


--
-- Name: chart_votes_user; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX chart_votes_user ON fanmade.chart_votes USING btree (user_id);


--
-- Name: charts_audio_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_audio_file ON fanmade.charts USING btree (audio_file);


--
-- Name: charts_bgimage_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_bgimage_file ON fanmade.charts USING btree (bgimage_file);


--
-- Name: charts_cover_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_cover_file ON fanmade.charts USING btree (cover_file);


--
-- Name: charts_movie_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_movie_file ON fanmade.charts USING btree (movie_file);


--
-- Name: charts_nijiiro_id; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE UNIQUE INDEX charts_nijiiro_id ON fanmade.charts USING btree (nijiiro_id) WHERE (nijiiro_id IS NOT NULL);


--
-- Name: charts_owners; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_owners ON fanmade.charts USING gin (owner_ids);


--
-- Name: charts_preview_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_preview_file ON fanmade.charts USING btree (preview_file);


--
-- Name: charts_published_recent; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_published_recent ON fanmade.charts USING btree (created_at DESC, id DESC) WHERE (status = 'published'::text);


--
-- Name: charts_tja_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX charts_tja_file ON fanmade.charts USING btree (tja_file);


--
-- Name: comment_votes_user; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX comment_votes_user ON fanmade.comment_votes USING btree (user_id);


--
-- Name: comments_author; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX comments_author ON fanmade.comments USING btree (author_id, created_at DESC);


--
-- Name: comments_chart_roots; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX comments_chart_roots ON fanmade.comments USING btree (chart_id, created_at DESC) WHERE (parent_id IS NULL);


--
-- Name: comments_dan_roots; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX comments_dan_roots ON fanmade.comments USING btree (dan_id, created_at DESC) WHERE ((parent_id IS NULL) AND (dan_id IS NOT NULL));


--
-- Name: comments_parent; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX comments_parent ON fanmade.comments USING btree (parent_id);


--
-- Name: comments_root; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX comments_root ON fanmade.comments USING btree (root_id);


--
-- Name: dan_attempts_course; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX dan_attempts_course ON fanmade.dan_attempts USING btree (course_id, submitted_at DESC);


--
-- Name: dan_attempts_player_recent; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX dan_attempts_player_recent ON ONLY net.dan_attempts USING btree (user_id, submitted_at DESC, id DESC);


--
-- Name: dan_attempts_user_id_submitted_at_id_idx; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX dan_attempts_user_id_submitted_at_id_idx ON fanmade.dan_attempts USING btree (user_id, submitted_at DESC, id DESC);


--
-- Name: dan_courses_code; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE UNIQUE INDEX dan_courses_code ON fanmade.dan_courses USING btree (code);


--
-- Name: dan_courses_listed; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX dan_courses_listed ON fanmade.dan_courses USING btree (official DESC, created_at DESC, id DESC) WHERE (status = 'published'::text);


--
-- Name: dan_courses_score; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX dan_courses_score ON fanmade.dan_courses USING btree (((upvotes - downvotes)) DESC) WHERE (status = 'published'::text);


--
-- Name: dan_votes_user; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX dan_votes_user ON fanmade.dan_votes USING btree (user_id);


--
-- Name: difficulty_constants_calibration; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX difficulty_constants_calibration ON fanmade.difficulty_constants USING btree (calibration_version);


--
-- Name: notifications_chart; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX notifications_chart ON fanmade.notifications USING btree (chart_id) WHERE (chart_id IS NOT NULL);


--
-- Name: notifications_collaboration; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE UNIQUE INDEX notifications_collaboration ON fanmade.notifications USING btree (collaboration_id) WHERE (collaboration_id IS NOT NULL);


--
-- Name: notifications_comment; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX notifications_comment ON fanmade.notifications USING btree (comment_id) WHERE (comment_id IS NOT NULL);


--
-- Name: notifications_dedupe; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE UNIQUE INDEX notifications_dedupe ON fanmade.notifications USING btree (recipient_id, dedupe_key) WHERE (dedupe_key IS NOT NULL);


--
-- Name: notifications_recipient; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX notifications_recipient ON fanmade.notifications USING btree (recipient_id, created_at DESC, id);


--
-- Name: notifications_unread; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX notifications_unread ON fanmade.notifications USING btree (recipient_id) WHERE (read_at IS NULL);


--
-- Name: playlist_favorites_user; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX playlist_favorites_user ON fanmade.playlist_favorites USING btree (user_id, favorited_at DESC);


--
-- Name: playlist_items_chart; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX playlist_items_chart ON fanmade.playlist_items USING btree (chart_id);


--
-- Name: playlist_items_order; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX playlist_items_order ON fanmade.playlist_items USING btree (playlist_id, "position", added_at, chart_id);


--
-- Name: playlists_cover_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX playlists_cover_file ON fanmade.playlists USING btree (cover_file);


--
-- Name: playlists_game_cover_file; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX playlists_game_cover_file ON fanmade.playlists USING btree (game_cover_file);


--
-- Name: playlists_owner; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX playlists_owner ON fanmade.playlists USING btree (owner_id, updated_at DESC, id);


--
-- Name: playlists_public; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX playlists_public ON fanmade.playlists USING btree (updated_at DESC, id) WHERE is_public;


--
-- Name: scores_leaderboard_current; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX scores_leaderboard_current ON fanmade.scores USING btree (difficulty_id, user_id, score DESC, submitted_at, id);


--
-- Name: scores_player_recent; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX scores_player_recent ON ONLY net.scores USING btree (user_id, submitted_at DESC, id DESC);


--
-- Name: scores_user_recent; Type: INDEX; Schema: fanmade; Owner: -
--

CREATE INDEX scores_user_recent ON fanmade.scores USING btree (user_id, submitted_at DESC, id DESC);


--
-- Name: awards_user; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX awards_user ON net.awards USING btree (user_id, created_at DESC);


--
-- Name: group_changes_user; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX group_changes_user ON net.group_changes USING btree (user_id, changed_at DESC);


--
-- Name: oidc_flows_expiry; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX oidc_flows_expiry ON net.oidc_flows USING btree (expires_at);


--
-- Name: sessions_expiry; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX sessions_expiry ON net.sessions USING btree (expires_at);


--
-- Name: titles_owner; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX titles_owner ON net.titles USING btree (owner_id) WHERE (owner_id IS NOT NULL);


--
-- Name: users_first_login; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX users_first_login ON net.users USING btree (first_login_at DESC NULLS LAST, id);


--
-- Name: users_in_groups; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX users_in_groups ON net.users USING btree (id) WHERE (groups <> 0);


--
-- Name: users_recent_activity; Type: INDEX; Schema: net; Owner: -
--

CREATE INDEX users_recent_activity ON net.users USING btree (last_active_at DESC NULLS LAST, id);


--
-- Name: chart_collaborations_inviter; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX chart_collaborations_inviter ON official.chart_collaborations USING btree (inviter_id, created_at);


--
-- Name: chart_collaborations_pending; Type: INDEX; Schema: official; Owner: -
--

CREATE UNIQUE INDEX chart_collaborations_pending ON official.chart_collaborations USING btree (chart_id, recipient_id) WHERE (status = 'pending'::text);


--
-- Name: chart_difficulties_content_hash; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX chart_difficulties_content_hash ON official.chart_difficulties USING btree (content_hash);


--
-- Name: chart_difficulties_course_level; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX chart_difficulties_course_level ON official.chart_difficulties USING btree (course, level, chart_id);


--
-- Name: chart_stats_score; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX chart_stats_score ON official.chart_stats USING btree (((upvotes - downvotes)) DESC);


--
-- Name: chart_votes_user; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX chart_votes_user ON official.chart_votes USING btree (user_id);


--
-- Name: charts_audio_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_audio_file ON official.charts USING btree (audio_file);


--
-- Name: charts_bgimage_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_bgimage_file ON official.charts USING btree (bgimage_file);


--
-- Name: charts_cover_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_cover_file ON official.charts USING btree (cover_file);


--
-- Name: charts_movie_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_movie_file ON official.charts USING btree (movie_file);


--
-- Name: charts_nijiiro_id; Type: INDEX; Schema: official; Owner: -
--

CREATE UNIQUE INDEX charts_nijiiro_id ON official.charts USING btree (nijiiro_id) WHERE (nijiiro_id IS NOT NULL);


--
-- Name: charts_official_id; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_official_id ON official.charts USING btree (official_id) WHERE (official_id IS NOT NULL);


--
-- Name: charts_owners; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_owners ON official.charts USING gin (owner_ids);


--
-- Name: charts_preview_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_preview_file ON official.charts USING btree (preview_file);


--
-- Name: charts_published_recent; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_published_recent ON official.charts USING btree (created_at DESC, id DESC) WHERE (status = 'published'::text);


--
-- Name: charts_tja_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX charts_tja_file ON official.charts USING btree (tja_file);


--
-- Name: comment_votes_user; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX comment_votes_user ON official.comment_votes USING btree (user_id);


--
-- Name: comments_author; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX comments_author ON official.comments USING btree (author_id, created_at DESC);


--
-- Name: comments_chart_roots; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX comments_chart_roots ON official.comments USING btree (chart_id, created_at DESC) WHERE (parent_id IS NULL);


--
-- Name: comments_dan_roots; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX comments_dan_roots ON official.comments USING btree (dan_id, created_at DESC) WHERE ((parent_id IS NULL) AND (dan_id IS NOT NULL));


--
-- Name: comments_parent; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX comments_parent ON official.comments USING btree (parent_id);


--
-- Name: comments_root; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX comments_root ON official.comments USING btree (root_id);


--
-- Name: dan_attempts_course; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX dan_attempts_course ON official.dan_attempts USING btree (course_id, submitted_at DESC);


--
-- Name: dan_attempts_user_id_submitted_at_id_idx; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX dan_attempts_user_id_submitted_at_id_idx ON official.dan_attempts USING btree (user_id, submitted_at DESC, id DESC);


--
-- Name: dan_courses_code; Type: INDEX; Schema: official; Owner: -
--

CREATE UNIQUE INDEX dan_courses_code ON official.dan_courses USING btree (code);


--
-- Name: dan_courses_listed; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX dan_courses_listed ON official.dan_courses USING btree (official DESC, created_at DESC, id DESC) WHERE (status = 'published'::text);


--
-- Name: dan_courses_score; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX dan_courses_score ON official.dan_courses USING btree (((upvotes - downvotes)) DESC) WHERE (status = 'published'::text);


--
-- Name: dan_votes_user; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX dan_votes_user ON official.dan_votes USING btree (user_id);


--
-- Name: difficulty_constants_calibration; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX difficulty_constants_calibration ON official.difficulty_constants USING btree (calibration_version);


--
-- Name: notifications_chart; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX notifications_chart ON official.notifications USING btree (chart_id) WHERE (chart_id IS NOT NULL);


--
-- Name: notifications_collaboration; Type: INDEX; Schema: official; Owner: -
--

CREATE UNIQUE INDEX notifications_collaboration ON official.notifications USING btree (collaboration_id) WHERE (collaboration_id IS NOT NULL);


--
-- Name: notifications_comment; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX notifications_comment ON official.notifications USING btree (comment_id) WHERE (comment_id IS NOT NULL);


--
-- Name: notifications_dedupe; Type: INDEX; Schema: official; Owner: -
--

CREATE UNIQUE INDEX notifications_dedupe ON official.notifications USING btree (recipient_id, dedupe_key) WHERE (dedupe_key IS NOT NULL);


--
-- Name: notifications_recipient; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX notifications_recipient ON official.notifications USING btree (recipient_id, created_at DESC);


--
-- Name: notifications_unread; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX notifications_unread ON official.notifications USING btree (recipient_id) WHERE (read_at IS NULL);


--
-- Name: playlist_favorites_user; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX playlist_favorites_user ON official.playlist_favorites USING btree (user_id, favorited_at DESC);


--
-- Name: playlist_items_chart; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX playlist_items_chart ON official.playlist_items USING btree (chart_id);


--
-- Name: playlist_items_order; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX playlist_items_order ON official.playlist_items USING btree (playlist_id, "position", added_at, chart_id);


--
-- Name: playlists_cover_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX playlists_cover_file ON official.playlists USING btree (cover_file);


--
-- Name: playlists_game_cover_file; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX playlists_game_cover_file ON official.playlists USING btree (game_cover_file);


--
-- Name: playlists_owner; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX playlists_owner ON official.playlists USING btree (owner_id, updated_at DESC, id);


--
-- Name: playlists_public; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX playlists_public ON official.playlists USING btree (updated_at DESC, id) WHERE is_public;


--
-- Name: scores_leaderboard_current; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX scores_leaderboard_current ON official.scores USING btree (difficulty_id, score DESC, submitted_at);


--
-- Name: scores_user; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX scores_user ON official.scores USING btree (user_id, submitted_at DESC);


--
-- Name: scores_user_id_submitted_at_id_idx; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX scores_user_id_submitted_at_id_idx ON official.scores USING btree (user_id, submitted_at DESC, id DESC);


--
-- Name: upload_requests_chart; Type: INDEX; Schema: official; Owner: -
--

CREATE INDEX upload_requests_chart ON official.upload_requests USING btree (chart_id);


--
-- Name: dan_attempts_source_id_key; Type: INDEX ATTACH; Schema: fanmade; Owner: -
--

ALTER INDEX net.dan_attempts_source_id_key ATTACH PARTITION fanmade.dan_attempts_source_id_key;


--
-- Name: dan_attempts_user_id_submitted_at_id_idx; Type: INDEX ATTACH; Schema: fanmade; Owner: -
--

ALTER INDEX net.dan_attempts_player_recent ATTACH PARTITION fanmade.dan_attempts_user_id_submitted_at_id_idx;


--
-- Name: scores_source_id_key; Type: INDEX ATTACH; Schema: fanmade; Owner: -
--

ALTER INDEX net.scores_source_id_key ATTACH PARTITION fanmade.scores_source_id_key;


--
-- Name: scores_user_recent; Type: INDEX ATTACH; Schema: fanmade; Owner: -
--

ALTER INDEX net.scores_player_recent ATTACH PARTITION fanmade.scores_user_recent;


--
-- Name: dan_attempts_source_id_key; Type: INDEX ATTACH; Schema: official; Owner: -
--

ALTER INDEX net.dan_attempts_source_id_key ATTACH PARTITION official.dan_attempts_source_id_key;


--
-- Name: dan_attempts_user_id_submitted_at_id_idx; Type: INDEX ATTACH; Schema: official; Owner: -
--

ALTER INDEX net.dan_attempts_player_recent ATTACH PARTITION official.dan_attempts_user_id_submitted_at_id_idx;


--
-- Name: scores_source_id_key; Type: INDEX ATTACH; Schema: official; Owner: -
--

ALTER INDEX net.scores_source_id_key ATTACH PARTITION official.scores_source_id_key;


--
-- Name: scores_user_id_submitted_at_id_idx; Type: INDEX ATTACH; Schema: official; Owner: -
--

ALTER INDEX net.scores_player_recent ATTACH PARTITION official.scores_user_id_submitted_at_id_idx;


--
-- Name: chart_difficulties chart_difficulties_invalidate_dan_songs; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER chart_difficulties_invalidate_dan_songs AFTER DELETE ON fanmade.chart_difficulties FOR EACH ROW EXECUTE FUNCTION fanmade.chart_difficulties_invalidate_dan_songs();


--
-- Name: chart_difficulties chart_difficulties_retire_scores; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER chart_difficulties_retire_scores AFTER UPDATE OF content_hash ON fanmade.chart_difficulties FOR EACH ROW WHEN (((old.content_hash IS NOT NULL) AND (old.content_hash IS DISTINCT FROM new.content_hash))) EXECUTE FUNCTION fanmade.chart_difficulties_retire_scores();


--
-- Name: chart_difficulties chart_difficulties_touch_chart; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER chart_difficulties_touch_chart AFTER INSERT OR DELETE OR UPDATE ON fanmade.chart_difficulties FOR EACH ROW EXECUTE FUNCTION fanmade.touch_related_chart();


--
-- Name: chart_resources chart_resources_touch_charts; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER chart_resources_touch_charts AFTER UPDATE ON fanmade.chart_resources FOR EACH ROW EXECUTE FUNCTION fanmade.chart_resources_touch_charts();


--
-- Name: chart_votes chart_votes_count; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER chart_votes_count AFTER INSERT OR DELETE OR UPDATE OF value ON fanmade.chart_votes FOR EACH ROW EXECUTE FUNCTION fanmade.count_chart_vote();


--
-- Name: chart_votes chart_votes_touch_chart; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER chart_votes_touch_chart AFTER INSERT OR DELETE OR UPDATE ON fanmade.chart_votes FOR EACH ROW EXECUTE FUNCTION fanmade.touch_related_chart();


--
-- Name: charts charts_check_owners; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER charts_check_owners BEFORE INSERT OR UPDATE OF owner_ids ON fanmade.charts FOR EACH ROW EXECUTE FUNCTION fanmade.check_chart_owners();


--
-- Name: charts charts_default_demo_end; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER charts_default_demo_end BEFORE INSERT ON fanmade.charts FOR EACH ROW EXECUTE FUNCTION fanmade.default_demo_end();


--
-- Name: charts charts_retire_files; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER charts_retire_files AFTER DELETE OR UPDATE OF tja_file, audio_file, cover_file, preview_file, movie_file, bgimage_file ON fanmade.charts FOR EACH ROW EXECUTE FUNCTION fanmade.retire_chart_files();


--
-- Name: charts charts_touch_update_at; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER charts_touch_update_at BEFORE UPDATE ON fanmade.charts FOR EACH ROW EXECUTE FUNCTION fanmade.charts_touch_update_at();


--
-- Name: comment_votes comment_votes_count; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER comment_votes_count AFTER INSERT OR DELETE OR UPDATE OF value ON fanmade.comment_votes FOR EACH ROW EXECUTE FUNCTION fanmade.count_comment_vote();


--
-- Name: comments comments_count; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER comments_count AFTER INSERT OR DELETE ON fanmade.comments FOR EACH ROW EXECUTE FUNCTION fanmade.count_comment();


--
-- Name: comments comments_touch_chart; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER comments_touch_chart AFTER INSERT OR DELETE OR UPDATE ON fanmade.comments FOR EACH ROW EXECUTE FUNCTION fanmade.touch_related_chart();


--
-- Name: dan_votes dan_votes_count; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER dan_votes_count AFTER INSERT OR DELETE OR UPDATE OF value ON fanmade.dan_votes FOR EACH ROW EXECUTE FUNCTION fanmade.count_dan_vote();


--
-- Name: playlists playlists_retire_cover; Type: TRIGGER; Schema: fanmade; Owner: -
--

CREATE TRIGGER playlists_retire_cover AFTER DELETE OR UPDATE OF cover_file, game_cover_file ON fanmade.playlists FOR EACH ROW EXECUTE FUNCTION fanmade.retire_playlist_cover();


--
-- Name: awards awards_ensure_player; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER awards_ensure_player BEFORE INSERT OR UPDATE OF user_id ON net.awards FOR EACH ROW EXECUTE FUNCTION net.ensure_player('user_id');


--
-- Name: users fanmade_users_protect_chart_owner; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER fanmade_users_protect_chart_owner BEFORE DELETE OR UPDATE OF id ON net.users FOR EACH ROW EXECUTE FUNCTION fanmade.protect_chart_owner();


--
-- Name: nameplates nameplates_ensure_player; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER nameplates_ensure_player BEFORE INSERT OR UPDATE OF user_id ON net.nameplates FOR EACH ROW EXECUTE FUNCTION net.ensure_player('user_id');


--
-- Name: users official_users_protect_chart_owner; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER official_users_protect_chart_owner BEFORE DELETE OR UPDATE OF id ON net.users FOR EACH ROW EXECUTE FUNCTION official.protect_chart_owner();


--
-- Name: sessions sessions_ensure_player; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER sessions_ensure_player BEFORE INSERT OR UPDATE OF user_id ON net.sessions FOR EACH ROW EXECUTE FUNCTION net.ensure_player('user_id');


--
-- Name: titles titles_ensure_player; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER titles_ensure_player BEFORE INSERT OR UPDATE OF owner_id ON net.titles FOR EACH ROW EXECUTE FUNCTION net.ensure_player('owner_id');


--
-- Name: user_dans user_dans_ensure_player; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER user_dans_ensure_player BEFORE INSERT OR UPDATE OF user_id ON net.user_dans FOR EACH ROW EXECUTE FUNCTION net.ensure_player('user_id');


--
-- Name: user_titles user_titles_ensure_player; Type: TRIGGER; Schema: net; Owner: -
--

CREATE TRIGGER user_titles_ensure_player BEFORE INSERT OR UPDATE OF user_id ON net.user_titles FOR EACH ROW EXECUTE FUNCTION net.ensure_player('user_id');


--
-- Name: chart_difficulties chart_difficulties_invalidate_dan_songs; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER chart_difficulties_invalidate_dan_songs AFTER DELETE ON official.chart_difficulties FOR EACH ROW EXECUTE FUNCTION official.chart_difficulties_invalidate_dan_songs();


--
-- Name: chart_difficulties chart_difficulties_retire_scores; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER chart_difficulties_retire_scores AFTER UPDATE OF content_hash ON official.chart_difficulties FOR EACH ROW WHEN (((old.content_hash IS NOT NULL) AND (old.content_hash IS DISTINCT FROM new.content_hash))) EXECUTE FUNCTION official.chart_difficulties_retire_scores();


--
-- Name: chart_difficulties chart_difficulties_touch_chart; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER chart_difficulties_touch_chart AFTER INSERT OR DELETE OR UPDATE ON official.chart_difficulties FOR EACH ROW EXECUTE FUNCTION official.touch_related_chart();


--
-- Name: chart_resources chart_resources_touch_charts; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER chart_resources_touch_charts AFTER UPDATE ON official.chart_resources FOR EACH ROW EXECUTE FUNCTION official.chart_resources_touch_charts();


--
-- Name: chart_votes chart_votes_count; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER chart_votes_count AFTER INSERT OR DELETE OR UPDATE OF value ON official.chart_votes FOR EACH ROW EXECUTE FUNCTION official.count_chart_vote();


--
-- Name: chart_votes chart_votes_touch_chart; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER chart_votes_touch_chart AFTER INSERT OR DELETE OR UPDATE ON official.chart_votes FOR EACH ROW EXECUTE FUNCTION official.touch_related_chart();


--
-- Name: charts charts_check_owners; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER charts_check_owners BEFORE INSERT OR UPDATE OF owner_ids ON official.charts FOR EACH ROW EXECUTE FUNCTION official.check_chart_owners();


--
-- Name: charts charts_retire_files; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER charts_retire_files AFTER DELETE OR UPDATE OF tja_file, audio_file, cover_file, preview_file, movie_file, bgimage_file ON official.charts FOR EACH ROW EXECUTE FUNCTION official.retire_chart_files();


--
-- Name: charts charts_retire_scores; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER charts_retire_scores AFTER UPDATE OF audio_file ON official.charts FOR EACH ROW WHEN ((old.audio_file IS DISTINCT FROM new.audio_file)) EXECUTE FUNCTION official.charts_retire_scores();


--
-- Name: charts charts_touch_update_at; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER charts_touch_update_at BEFORE UPDATE ON official.charts FOR EACH ROW EXECUTE FUNCTION official.charts_touch_update_at();


--
-- Name: comment_votes comment_votes_count; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER comment_votes_count AFTER INSERT OR DELETE OR UPDATE OF value ON official.comment_votes FOR EACH ROW EXECUTE FUNCTION official.count_comment_vote();


--
-- Name: comments comments_count; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER comments_count AFTER INSERT OR DELETE ON official.comments FOR EACH ROW EXECUTE FUNCTION official.count_comment();


--
-- Name: comments comments_touch_chart; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER comments_touch_chart AFTER INSERT OR DELETE OR UPDATE ON official.comments FOR EACH ROW EXECUTE FUNCTION official.touch_related_chart();


--
-- Name: dan_votes dan_votes_count; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER dan_votes_count AFTER INSERT OR DELETE OR UPDATE OF value ON official.dan_votes FOR EACH ROW EXECUTE FUNCTION official.count_dan_vote();


--
-- Name: playlists playlists_retire_cover; Type: TRIGGER; Schema: official; Owner: -
--

CREATE TRIGGER playlists_retire_cover AFTER DELETE OR UPDATE OF cover_file, game_cover_file ON official.playlists FOR EACH ROW EXECUTE FUNCTION official.retire_playlist_cover();


--
-- Name: chart_collaborations chart_collaborations_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_collaborations
    ADD CONSTRAINT chart_collaborations_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE CASCADE;


--
-- Name: chart_collaborations chart_collaborations_inviter_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_collaborations
    ADD CONSTRAINT chart_collaborations_inviter_id_fkey FOREIGN KEY (inviter_id) REFERENCES net.users(id);


--
-- Name: chart_collaborations chart_collaborations_recipient_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_collaborations
    ADD CONSTRAINT chart_collaborations_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES net.users(id);


--
-- Name: chart_collaborations chart_collaborations_revoked_by_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_collaborations
    ADD CONSTRAINT chart_collaborations_revoked_by_fkey FOREIGN KEY (revoked_by) REFERENCES net.users(id);


--
-- Name: chart_difficulties chart_difficulties_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_difficulties
    ADD CONSTRAINT chart_difficulties_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE CASCADE;


--
-- Name: chart_stats chart_stats_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_stats
    ADD CONSTRAINT chart_stats_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE CASCADE;


--
-- Name: chart_votes chart_votes_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_votes
    ADD CONSTRAINT chart_votes_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE CASCADE;


--
-- Name: chart_votes chart_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.chart_votes
    ADD CONSTRAINT chart_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: charts charts_audio_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_audio_file_fkey FOREIGN KEY (audio_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: charts charts_bgimage_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_bgimage_file_fkey FOREIGN KEY (bgimage_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: charts charts_cover_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_cover_file_fkey FOREIGN KEY (cover_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: charts charts_movie_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_movie_file_fkey FOREIGN KEY (movie_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: charts charts_preview_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_preview_file_fkey FOREIGN KEY (preview_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: charts charts_tja_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.charts
    ADD CONSTRAINT charts_tja_file_fkey FOREIGN KEY (tja_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: comment_votes comment_votes_comment_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comment_votes
    ADD CONSTRAINT comment_votes_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES fanmade.comments(id) ON DELETE CASCADE;


--
-- Name: comment_votes comment_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comment_votes
    ADD CONSTRAINT comment_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: comments comments_author_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comments
    ADD CONSTRAINT comments_author_id_fkey FOREIGN KEY (author_id) REFERENCES net.users(id);


--
-- Name: comments comments_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comments
    ADD CONSTRAINT comments_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE CASCADE;


--
-- Name: comments comments_dan_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comments
    ADD CONSTRAINT comments_dan_id_fkey FOREIGN KEY (dan_id) REFERENCES fanmade.dan_courses(id) ON DELETE CASCADE;


--
-- Name: comments comments_parent_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.comments
    ADD CONSTRAINT comments_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES fanmade.comments(id) ON DELETE CASCADE;


--
-- Name: dan_attempts dan_attempts_course_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_attempts
    ADD CONSTRAINT dan_attempts_course_id_fkey FOREIGN KEY (course_id) REFERENCES fanmade.dan_courses(id);


--
-- Name: dan_attempts dan_attempts_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_attempts
    ADD CONSTRAINT dan_attempts_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: dan_course_songs dan_course_songs_course_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_course_songs
    ADD CONSTRAINT dan_course_songs_course_id_fkey FOREIGN KEY (course_id) REFERENCES fanmade.dan_courses(id) ON DELETE CASCADE;


--
-- Name: dan_courses dan_courses_owner_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_courses
    ADD CONSTRAINT dan_courses_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES net.users(id);


--
-- Name: dan_votes dan_votes_dan_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_votes
    ADD CONSTRAINT dan_votes_dan_id_fkey FOREIGN KEY (dan_id) REFERENCES fanmade.dan_courses(id) ON DELETE CASCADE;


--
-- Name: dan_votes dan_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.dan_votes
    ADD CONSTRAINT dan_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: difficulty_constants difficulty_constants_calibration_version_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.difficulty_constants
    ADD CONSTRAINT difficulty_constants_calibration_version_fkey FOREIGN KEY (calibration_version) REFERENCES fanmade.constants_calibrations(version);


--
-- Name: notification_mutes notification_mutes_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notification_mutes
    ADD CONSTRAINT notification_mutes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: notifications notifications_actor_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notifications
    ADD CONSTRAINT notifications_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES net.users(id);


--
-- Name: notifications notifications_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notifications
    ADD CONSTRAINT notifications_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_collaboration_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notifications
    ADD CONSTRAINT notifications_collaboration_id_fkey FOREIGN KEY (collaboration_id) REFERENCES fanmade.chart_collaborations(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_comment_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notifications
    ADD CONSTRAINT notifications_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES fanmade.comments(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_dan_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notifications
    ADD CONSTRAINT notifications_dan_id_fkey FOREIGN KEY (dan_id) REFERENCES fanmade.dan_courses(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_recipient_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.notifications
    ADD CONSTRAINT notifications_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES net.users(id);


--
-- Name: playlist_favorites playlist_favorites_playlist_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlist_favorites
    ADD CONSTRAINT playlist_favorites_playlist_id_fkey FOREIGN KEY (playlist_id) REFERENCES fanmade.playlists(id) ON DELETE CASCADE;


--
-- Name: playlist_favorites playlist_favorites_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlist_favorites
    ADD CONSTRAINT playlist_favorites_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: playlist_items playlist_items_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlist_items
    ADD CONSTRAINT playlist_items_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE CASCADE;


--
-- Name: playlist_items playlist_items_playlist_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlist_items
    ADD CONSTRAINT playlist_items_playlist_id_fkey FOREIGN KEY (playlist_id) REFERENCES fanmade.playlists(id) ON DELETE CASCADE;


--
-- Name: playlists playlists_cover_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlists
    ADD CONSTRAINT playlists_cover_file_fkey FOREIGN KEY (cover_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: playlists playlists_game_cover_file_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlists
    ADD CONSTRAINT playlists_game_cover_file_fkey FOREIGN KEY (game_cover_file) REFERENCES fanmade.chart_resources(id);


--
-- Name: playlists playlists_owner_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.playlists
    ADD CONSTRAINT playlists_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES net.users(id);


--
-- Name: retired_score_requests retired_score_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.retired_score_requests
    ADD CONSTRAINT retired_score_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id) ON DELETE CASCADE;


--
-- Name: scores scores_difficulty_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.scores
    ADD CONSTRAINT scores_difficulty_id_fkey FOREIGN KEY (difficulty_id) REFERENCES fanmade.chart_difficulties(id);


--
-- Name: scores scores_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.scores
    ADD CONSTRAINT scores_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: upload_requests upload_requests_chart_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.upload_requests
    ADD CONSTRAINT upload_requests_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES fanmade.charts(id) ON DELETE SET NULL;


--
-- Name: upload_requests upload_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: fanmade; Owner: -
--

ALTER TABLE ONLY fanmade.upload_requests
    ADD CONSTRAINT upload_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: awards awards_user_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.awards
    ADD CONSTRAINT awards_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: group_changes group_changes_changed_by_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.group_changes
    ADD CONSTRAINT group_changes_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES net.users(id);


--
-- Name: group_changes group_changes_user_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.group_changes
    ADD CONSTRAINT group_changes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id) ON DELETE CASCADE;


--
-- Name: nameplates nameplates_title_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.nameplates
    ADD CONSTRAINT nameplates_title_id_fkey FOREIGN KEY (title_id) REFERENCES net.titles(id) ON DELETE SET NULL;


--
-- Name: nameplates nameplates_user_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.nameplates
    ADD CONSTRAINT nameplates_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: sessions sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id) ON DELETE CASCADE;


--
-- Name: titles titles_owner_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.titles
    ADD CONSTRAINT titles_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES net.users(id);


--
-- Name: user_dans user_dans_award_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.user_dans
    ADD CONSTRAINT user_dans_award_id_fkey FOREIGN KEY (award_id) REFERENCES net.awards(id) ON DELETE SET NULL;


--
-- Name: user_dans user_dans_user_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.user_dans
    ADD CONSTRAINT user_dans_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: user_titles user_titles_award_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.user_titles
    ADD CONSTRAINT user_titles_award_id_fkey FOREIGN KEY (award_id) REFERENCES net.awards(id) ON DELETE SET NULL;


--
-- Name: user_titles user_titles_title_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.user_titles
    ADD CONSTRAINT user_titles_title_id_fkey FOREIGN KEY (title_id) REFERENCES net.titles(id) ON DELETE CASCADE;


--
-- Name: user_titles user_titles_user_id_fkey; Type: FK CONSTRAINT; Schema: net; Owner: -
--

ALTER TABLE ONLY net.user_titles
    ADD CONSTRAINT user_titles_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: chart_collaborations chart_collaborations_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_collaborations
    ADD CONSTRAINT chart_collaborations_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE CASCADE;


--
-- Name: chart_collaborations chart_collaborations_inviter_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_collaborations
    ADD CONSTRAINT chart_collaborations_inviter_id_fkey FOREIGN KEY (inviter_id) REFERENCES net.users(id);


--
-- Name: chart_collaborations chart_collaborations_recipient_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_collaborations
    ADD CONSTRAINT chart_collaborations_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES net.users(id);


--
-- Name: chart_collaborations chart_collaborations_revoked_by_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_collaborations
    ADD CONSTRAINT chart_collaborations_revoked_by_fkey FOREIGN KEY (revoked_by) REFERENCES net.users(id);


--
-- Name: chart_difficulties chart_difficulties_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_difficulties
    ADD CONSTRAINT chart_difficulties_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE CASCADE;


--
-- Name: chart_stats chart_stats_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_stats
    ADD CONSTRAINT chart_stats_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE CASCADE;


--
-- Name: chart_votes chart_votes_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_votes
    ADD CONSTRAINT chart_votes_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE CASCADE;


--
-- Name: chart_votes chart_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.chart_votes
    ADD CONSTRAINT chart_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: charts charts_audio_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_audio_file_fkey FOREIGN KEY (audio_file) REFERENCES official.chart_resources(id) ON DELETE RESTRICT;


--
-- Name: charts charts_bgimage_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_bgimage_file_fkey FOREIGN KEY (bgimage_file) REFERENCES official.chart_resources(id) ON DELETE RESTRICT;


--
-- Name: charts charts_cover_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_cover_file_fkey FOREIGN KEY (cover_file) REFERENCES official.chart_resources(id) ON DELETE RESTRICT;


--
-- Name: charts charts_movie_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_movie_file_fkey FOREIGN KEY (movie_file) REFERENCES official.chart_resources(id) ON DELETE RESTRICT;


--
-- Name: charts charts_preview_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_preview_file_fkey FOREIGN KEY (preview_file) REFERENCES official.chart_resources(id) ON DELETE RESTRICT;


--
-- Name: charts charts_tja_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.charts
    ADD CONSTRAINT charts_tja_file_fkey FOREIGN KEY (tja_file) REFERENCES official.chart_resources(id) ON DELETE RESTRICT;


--
-- Name: comment_votes comment_votes_comment_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comment_votes
    ADD CONSTRAINT comment_votes_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES official.comments(id) ON DELETE CASCADE;


--
-- Name: comment_votes comment_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comment_votes
    ADD CONSTRAINT comment_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: comments comments_author_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comments
    ADD CONSTRAINT comments_author_id_fkey FOREIGN KEY (author_id) REFERENCES net.users(id);


--
-- Name: comments comments_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comments
    ADD CONSTRAINT comments_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE CASCADE;


--
-- Name: comments comments_dan_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comments
    ADD CONSTRAINT comments_dan_id_fkey FOREIGN KEY (dan_id) REFERENCES official.dan_courses(id) ON DELETE CASCADE;


--
-- Name: comments comments_parent_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.comments
    ADD CONSTRAINT comments_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES official.comments(id) ON DELETE CASCADE;


--
-- Name: dan_attempts dan_attempts_course_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_attempts
    ADD CONSTRAINT dan_attempts_course_id_fkey FOREIGN KEY (course_id) REFERENCES official.dan_courses(id);


--
-- Name: dan_attempts dan_attempts_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_attempts
    ADD CONSTRAINT dan_attempts_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: dan_course_songs dan_course_songs_course_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_course_songs
    ADD CONSTRAINT dan_course_songs_course_id_fkey FOREIGN KEY (course_id) REFERENCES official.dan_courses(id) ON DELETE CASCADE;


--
-- Name: dan_courses dan_courses_owner_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_courses
    ADD CONSTRAINT dan_courses_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES net.users(id);


--
-- Name: dan_votes dan_votes_dan_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_votes
    ADD CONSTRAINT dan_votes_dan_id_fkey FOREIGN KEY (dan_id) REFERENCES official.dan_courses(id) ON DELETE CASCADE;


--
-- Name: dan_votes dan_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.dan_votes
    ADD CONSTRAINT dan_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: difficulty_constants difficulty_constants_calibration_version_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.difficulty_constants
    ADD CONSTRAINT difficulty_constants_calibration_version_fkey FOREIGN KEY (calibration_version) REFERENCES official.constants_calibrations(version);


--
-- Name: notification_mutes notification_mutes_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notification_mutes
    ADD CONSTRAINT notification_mutes_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: notifications notifications_actor_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notifications
    ADD CONSTRAINT notifications_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES net.users(id);


--
-- Name: notifications notifications_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notifications
    ADD CONSTRAINT notifications_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_collaboration_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notifications
    ADD CONSTRAINT notifications_collaboration_id_fkey FOREIGN KEY (collaboration_id) REFERENCES official.chart_collaborations(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_comment_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notifications
    ADD CONSTRAINT notifications_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES official.comments(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_dan_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notifications
    ADD CONSTRAINT notifications_dan_id_fkey FOREIGN KEY (dan_id) REFERENCES official.dan_courses(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_recipient_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.notifications
    ADD CONSTRAINT notifications_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES net.users(id);


--
-- Name: playlist_favorites playlist_favorites_playlist_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlist_favorites
    ADD CONSTRAINT playlist_favorites_playlist_id_fkey FOREIGN KEY (playlist_id) REFERENCES official.playlists(id) ON DELETE CASCADE;


--
-- Name: playlist_favorites playlist_favorites_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlist_favorites
    ADD CONSTRAINT playlist_favorites_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: playlist_items playlist_items_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlist_items
    ADD CONSTRAINT playlist_items_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE CASCADE;


--
-- Name: playlist_items playlist_items_playlist_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlist_items
    ADD CONSTRAINT playlist_items_playlist_id_fkey FOREIGN KEY (playlist_id) REFERENCES official.playlists(id) ON DELETE CASCADE;


--
-- Name: playlists playlists_cover_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlists
    ADD CONSTRAINT playlists_cover_file_fkey FOREIGN KEY (cover_file) REFERENCES official.chart_resources(id);


--
-- Name: playlists playlists_game_cover_file_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlists
    ADD CONSTRAINT playlists_game_cover_file_fkey FOREIGN KEY (game_cover_file) REFERENCES official.chart_resources(id);


--
-- Name: playlists playlists_owner_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.playlists
    ADD CONSTRAINT playlists_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES net.users(id);


--
-- Name: retired_score_requests retired_score_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.retired_score_requests
    ADD CONSTRAINT retired_score_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id) ON DELETE CASCADE;


--
-- Name: scores scores_difficulty_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.scores
    ADD CONSTRAINT scores_difficulty_id_fkey FOREIGN KEY (difficulty_id) REFERENCES official.chart_difficulties(id);


--
-- Name: scores scores_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.scores
    ADD CONSTRAINT scores_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- Name: upload_requests upload_requests_chart_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.upload_requests
    ADD CONSTRAINT upload_requests_chart_id_fkey FOREIGN KEY (chart_id) REFERENCES official.charts(id) ON DELETE SET NULL;


--
-- Name: upload_requests upload_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: official; Owner: -
--

ALTER TABLE ONLY official.upload_requests
    ADD CONSTRAINT upload_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES net.users(id);


--
-- PostgreSQL database dump complete
--


-- Rows of the baseline
INSERT INTO net.schema_migrations (version) VALUES (1);
