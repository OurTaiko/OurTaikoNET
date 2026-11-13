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
  LOOP
    EXECUTE replace(definition, 'fanmade', 'official');
  END LOOP;
  IF (SELECT count(*) FROM pg_trigger WHERE tgname IN ('charts_retire_files', 'playlists_retire_cover') AND NOT tgisinternal) <> 4 THEN
    RAISE EXCEPTION 'the file-retiring triggers were not copied to the official source';
  END IF;
END $$;
