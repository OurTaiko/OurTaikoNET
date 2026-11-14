-- What the account center publishes about a player, kept here so that showing a name does
-- not ask the account center: the nickname and the address of the avatar as it gave them
-- (empty: none, or a player it no longer knows), and when it was last asked. The account
-- center remains where they are changed; these are a copy (internal/account, profiles.go).
ALTER TABLE net.users
  ADD COLUMN nickname text NOT NULL DEFAULT '',
  ADD COLUMN avatar_url text NOT NULL DEFAULT '',
  ADD COLUMN profile_checked_at timestamptz;

COMMENT ON COLUMN net.users.nickname IS 'Copy of the account center''s nickname; empty when it has none for this player.';
COMMENT ON COLUMN net.users.avatar_url IS 'Copy of the account center''s avatar address; empty when the player has none.';
COMMENT ON COLUMN net.users.profile_checked_at IS 'When the account center was last asked about this player; null: never.';
