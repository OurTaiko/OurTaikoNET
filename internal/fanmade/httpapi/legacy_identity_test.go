package httpapi

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests were written for Fanmade when it still kept accounts of its own. Their fixtures
// insert users with a name, password and administrator flag; testServer then copies those
// rows aside as what the fake account center knows and takes the columns away, which is what
// Fanmade's migration to the account center did. From there on the tables are the real ones.
//
// ourtaiko_net never had those columns, so a test database gets them for the time before
// testServer, and dropLegacyIdentity stands in for that migration.
//
// To do: write the fixtures against the fake account center directly and remove this file.

// addLegacyIdentity gives net.users and net.sessions the shape Fanmade's fixtures insert into.
func addLegacyIdentity(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
ALTER TABLE net.users ADD COLUMN username text NOT NULL, ADD COLUMN password_hash text NOT NULL,
 ADD COLUMN email text, ADD COLUMN email_verified_at timestamptz,
 ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(), ADD COLUMN is_admin boolean NOT NULL DEFAULT false,
 ADD COLUMN nickname text NOT NULL,
 ADD CONSTRAINT users_username_key UNIQUE (username);
CREATE FUNCTION net.prepare_user_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.nickname IS NULL THEN NEW.nickname := NEW.username; END IF;
 ELSIF NEW.username IS NOT DISTINCT FROM OLD.username THEN
  RETURN NEW;
 END IF;
 IF NEW.username !~ '^[A-Za-z0-9]{3,24}$' THEN
  RAISE EXCEPTION 'username must contain 3-24 ASCII letters or digits' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER users_prepare_profile BEFORE INSERT OR UPDATE OF username ON net.users
 FOR EACH ROW EXECUTE FUNCTION net.prepare_user_profile();
ALTER TABLE net.sessions ALTER COLUMN upstream_token DROP NOT NULL`)
	return err
}

// dropLegacyIdentity returns net.users and net.sessions to what ourtaiko_net really has.
// Calling it again changes nothing.
func dropLegacyIdentity(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
DROP TRIGGER IF EXISTS users_prepare_profile ON net.users;
DROP FUNCTION IF EXISTS net.prepare_user_profile();
ALTER TABLE net.users DROP COLUMN IF EXISTS username, DROP COLUMN IF EXISTS password_hash, DROP COLUMN IF EXISTS nickname,
 DROP COLUMN IF EXISTS email, DROP COLUMN IF EXISTS email_verified_at, DROP COLUMN IF EXISTS is_admin, DROP COLUMN IF EXISTS created_at;
DELETE FROM net.sessions WHERE upstream_token IS NULL;
ALTER TABLE net.sessions ALTER COLUMN upstream_token SET NOT NULL`)
	return err
}
