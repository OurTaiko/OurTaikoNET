-- The playlists the home page recommends (internal/profile/httpapi/featured.go): a few public
-- playlists of either song source, in the order the site's administrator put them, each with a
-- line of the administrator's own. The playlists themselves stay in their source; a row here
-- names one by its source and its id there, which no foreign key can say, so the triggers
-- below keep the promise instead: a recommended playlist exists and is public.
CREATE TABLE IF NOT EXISTS net.featured_playlists (
    source text NOT NULL CHECK (source IN ('fanmade', 'official')),
    playlist_id text NOT NULL,
    -- Where it stands on the home page, counted from 0.
    position integer NOT NULL UNIQUE CHECK (position >= 0),
    -- Why it is recommended, in the administrator's words; empty for none.
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 40 AND note = btrim(note)),
    changed_by text NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, playlist_id)
);

COMMENT ON TABLE net.featured_playlists IS 'Playlists the home page recommends, chosen and ordered by the site''s administrator; each names a public playlist of a song source.';

-- A playlist that is deleted or made private is no longer recommended.
CREATE OR REPLACE FUNCTION net.unfeature_playlist() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.is_public THEN
    RETURN NULL;
  END IF;
  DELETE FROM featured_playlists WHERE source = TG_TABLE_SCHEMA AND playlist_id = OLD.id;
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS playlists_unfeature ON fanmade.playlists;
CREATE TRIGGER playlists_unfeature AFTER DELETE OR UPDATE OF is_public ON fanmade.playlists FOR EACH ROW EXECUTE FUNCTION net.unfeature_playlist();
DROP TRIGGER IF EXISTS playlists_unfeature ON official.playlists;
CREATE TRIGGER playlists_unfeature AFTER DELETE OR UPDATE OF is_public ON official.playlists FOR EACH ROW EXECUTE FUNCTION net.unfeature_playlist();
