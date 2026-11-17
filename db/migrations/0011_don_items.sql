-- The 3D Don's parts a player owns (internal/donitems). The parts themselves are in the public
-- catalog (3dchara/catalog.json), not here. Every part may be worn by anyone unless it is locked
-- (don_item_locks): a locked part is worn only by those who own it (user_don_items), given by an
-- administrator or a rule. Locking a part gives it to everyone wearing it then, so nobody's Don
-- changes. Number 0 of cos, head and body takes the piece off, and 0 of puchi and paint is none:
-- those are never locked or owned.
CREATE TABLE IF NOT EXISTS net.don_item_locks (
    part text NOT NULL CHECK (part IN ('cos', 'head', 'body', 'puchi', 'paint')),
    item_id integer NOT NULL CHECK (item_id > 0),
    locked_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (part, item_id)
);

CREATE TABLE IF NOT EXISTS net.user_don_items (
    user_id text NOT NULL REFERENCES net.users (id) ON DELETE CASCADE,
    part text NOT NULL CHECK (part IN ('cos', 'head', 'body', 'puchi', 'paint')),
    item_id integer NOT NULL CHECK (item_id > 0),
    -- Why it was given (an administrator's grant, a rule); none for one given on locking.
    award_id bigint REFERENCES net.awards (id) ON DELETE SET NULL,
    acquired_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, part, item_id)
);

COMMENT ON TABLE net.don_item_locks IS 'Parts of the 3D Don that must be owned (user_don_items) to be worn; every other part of the catalog is for everyone.';
COMMENT ON TABLE net.user_don_items IS 'Parts of the 3D Don a player owns: given by an administrator or a rule (award_id), or because they wore it when it was locked.';

-- A part given is a reward (internal/rewards), as a title is. The catalog is not in the database,
-- so the reward names the part; clients read its name and picture from the catalog.
CREATE OR REPLACE FUNCTION net.reward_don_item() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  -- Given because it was worn when locked: nothing new to tell.
  IF NEW.award_id IS NULL THEN
    RETURN NULL;
  END IF;
  INSERT INTO rewards(user_id, kind, data)
  SELECT NEW.user_id, 'don_item', jsonb_build_object('part', NEW.part, 'itemId', NEW.item_id,
    'service', a.service, 'courseTitle', a.course_title)
  FROM awards a WHERE a.id = NEW.award_id;
  RETURN NULL;
END $$;

-- A part taken back before it was seen is not shown.
CREATE OR REPLACE FUNCTION net.unreward_don_item() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  DELETE FROM rewards WHERE user_id = OLD.user_id AND kind = 'don_item' AND seen_at IS NULL
    AND data ->> 'part' = OLD.part AND (data ->> 'itemId')::integer = OLD.item_id;
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS user_don_items_reward ON net.user_don_items;
CREATE TRIGGER user_don_items_reward AFTER INSERT ON net.user_don_items FOR EACH ROW EXECUTE FUNCTION net.reward_don_item();
DROP TRIGGER IF EXISTS user_don_items_unreward ON net.user_don_items;
CREATE TRIGGER user_don_items_unreward AFTER DELETE ON net.user_don_items FOR EACH ROW EXECUTE FUNCTION net.unreward_don_item();
