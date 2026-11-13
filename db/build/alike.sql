-- What one source's old database did by itself and the other's did not, given to both, so
-- that one program can serve the two sources the same way. Run after both are adopted.
\set ON_ERROR_STOP 1

-- Files nothing refers to any more. Fanmade's database queues a chart's or playlist's file
-- for deletion (retired_files) in the very transaction that replaces or deletes the row that
-- referred to it, and the program deletes what is queued. OurTaiko Database left it to its
-- program to delete such files after the change, with nothing to find them again if it
-- stopped in between. The official source now queues them too, by the same functions.
DO $$
DECLARE
  name text;
  definition text;
BEGIN
  FOREACH name IN ARRAY ARRAY['retire_chart_files', 'retire_playlist_cover'] LOOP
    EXECUTE replace(pg_get_functiondef(('fanmade.' || name)::regproc), 'fanmade', 'official');
  END LOOP;
  FOR definition IN
    SELECT pg_get_triggerdef(t.oid) FROM pg_trigger t
     WHERE (t.tgrelid, t.tgname) IN (('fanmade.charts'::regclass, 'charts_retire_files'), ('fanmade.playlists'::regclass, 'playlists_retire_cover'))
       -- Not again where this script has run before.
       AND NOT EXISTS (SELECT 1 FROM pg_trigger o WHERE o.tgname = t.tgname AND o.tgrelid = replace(t.tgrelid::regclass::text, 'fanmade', 'official')::regclass)
  LOOP
    EXECUTE replace(definition, 'fanmade', 'official');
  END LOOP;
  IF (SELECT count(*) FROM pg_trigger WHERE tgname IN ('charts_retire_files', 'playlists_retire_cover') AND NOT tgisinternal) <> 4 THEN
    RAISE EXCEPTION 'the file-retiring triggers were not copied to the official source';
  END IF;
END $$;

-- Scores played on an audio that was replaced. A play belongs to the files it was played on:
-- when a chart gets another audio, its scores stop being those of the current files
-- (is_latest). OurTaiko Database did this by trigger whenever charts.audio_file changed.
-- Fanmade left it to its program, which compared the audio's content, because it also moves
-- a chart to another record of the same audio (old files stored again under their content's
-- name). Both sources now do it by one trigger, which compares content.
DO $block$
DECLARE
  source text;
BEGIN
  FOREACH source IN ARRAY ARRAY['fanmade', 'official'] LOOP
    EXECUTE format($f$CREATE OR REPLACE FUNCTION %1$I.charts_retire_scores() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO %1$L, 'net'
    AS $body$
BEGIN
  IF (SELECT sha256 FROM %1$I.chart_resources WHERE id = OLD.audio_file)
     IS DISTINCT FROM (SELECT sha256 FROM %1$I.chart_resources WHERE id = NEW.audio_file) THEN
    UPDATE %1$I.scores SET is_latest = false
     WHERE is_latest AND difficulty_id IN (SELECT id FROM %1$I.chart_difficulties WHERE chart_id = NEW.id);
  END IF;
  RETURN NULL;
END;
$body$ $f$, source);
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = format('%I.charts', source)::regclass AND tgname = 'charts_retire_scores') THEN
      EXECUTE format('CREATE TRIGGER charts_retire_scores AFTER UPDATE OF audio_file ON %1$I.charts FOR EACH ROW WHEN (old.audio_file IS DISTINCT FROM new.audio_file) EXECUTE FUNCTION %1$I.charts_retire_scores()', source);
    END IF;
  END LOOP;
END $block$;

-- A song's ID in the official game. Only the official source's songs have one, but both
-- sources' charts are read and searched by the same statements, so both tables have the
-- column; on fanmade it stays empty.
ALTER TABLE fanmade.charts ADD COLUMN IF NOT EXISTS official_id text;
CREATE INDEX IF NOT EXISTS charts_official_id ON fanmade.charts (official_id) WHERE official_id IS NOT NULL;
