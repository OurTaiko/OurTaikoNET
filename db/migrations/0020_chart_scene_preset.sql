-- A chart's own play backdrop (stage preset): what its TJA says in SCENEPRESET, read by
-- TJAParser when the file is stored, and changed on the site by rewriting that header, so the
-- file stays what the game reads. NULL until the stored file has been read for it (charts from
-- before this column are filled at startup) and for a song without files; empty when the file
-- names none, or a name this site does not take. Like a playlist's, it is not checked against
-- the published presets.
ALTER TABLE fanmade.charts ADD COLUMN IF NOT EXISTS scene_preset text
    CONSTRAINT charts_scene_preset_check CHECK (scene_preset ~ '^[A-Za-z0-9_]{0,32}$');
ALTER TABLE official.charts ADD COLUMN IF NOT EXISTS scene_preset text
    CONSTRAINT charts_scene_preset_check CHECK (scene_preset ~ '^[A-Za-z0-9_]{0,32}$');
COMMENT ON COLUMN fanmade.charts.scene_preset IS 'The stage preset the chart''s TJA names (SCENEPRESET); empty for none, NULL while the stored file has not been read for it.';
COMMENT ON COLUMN official.charts.scene_preset IS 'The stage preset the chart''s TJA names (SCENEPRESET); empty for none, NULL while the stored file has not been read for it.';

-- scene_preset repeats what tja_file says, so it never changes a chart by itself: an upload or
-- an edit of the preset replaces tja_file in the same statement, and filling it for an older
-- chart is only reading the file. update_at therefore does not look at it.
CREATE OR REPLACE FUNCTION fanmade.charts_touch_update_at() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'fanmade', 'net'
    AS $$
BEGIN
  IF to_jsonb(NEW) - 'update_at' - 'preview_file' - 'scene_preset' IS DISTINCT FROM to_jsonb(OLD) - 'update_at' - 'preview_file' - 'scene_preset' THEN
    NEW.update_at := greatest(OLD.update_at, clock_timestamp());
  ELSE
    -- Only update_at, the preview clip or the preset read from the file was written.
    NEW.update_at := greatest(OLD.update_at, NEW.update_at);
  END IF;
  RETURN NEW;
END;
$$;
CREATE OR REPLACE FUNCTION official.charts_touch_update_at() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'official', 'net'
    AS $$
BEGIN
  IF to_jsonb(NEW) - 'update_at' - 'preview_file' - 'scene_preset' IS DISTINCT FROM to_jsonb(OLD) - 'update_at' - 'preview_file' - 'scene_preset' THEN
    NEW.update_at := greatest(OLD.update_at, clock_timestamp());
  ELSE
    -- Only update_at, the preview clip or the preset read from the file was written.
    NEW.update_at := greatest(OLD.update_at, NEW.update_at);
  END IF;
  RETURN NEW;
END;
$$;
