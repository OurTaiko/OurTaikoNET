-- The visit count (internal/visits) keeps each page view with the moment it was made, so that
-- whoever reads it sees days of their own time zone; until now a day was one of UTC+8 and only
-- its totals were kept (0005).
--
-- Still no cookie and no address. A visitor is told from another by a hash of their address and
-- browser with a salt, and a salt lives for a span of two UTC days only: when the span is over the
-- salt is deleted and the hashes it made can no longer be traced to anyone, though they still
-- tell the visitors of that span apart. Spans start every day, so every moment lies in two of
-- them, one beginning on an even day and one on an odd day (counted from 1970-01-01), and each
-- view keeps a hash for both. Any day of any time zone lies within one span, the one beginning on
-- the UTC day where it begins, so its visitors are counted by that span's hashes.
CREATE TABLE IF NOT EXISTS net.visit_views (
    at timestamptz NOT NULL,
    page text NOT NULL,
    visitor_even bytea NOT NULL CHECK (length(visitor_even) = 32),
    visitor_odd bytea NOT NULL CHECK (length(visitor_odd) = 32)
);
CREATE INDEX IF NOT EXISTS visit_views_at ON net.visit_views (at);

CREATE TABLE IF NOT EXISTS net.visit_salts (
    span date NOT NULL PRIMARY KEY,
    salt bytea NOT NULL CHECK (length(salt) = 16)
);

COMMENT ON TABLE net.visit_views IS 'Page views of the website, with the moment each was made. A page is an address with its ids replaced by :id. visitor_even and visitor_odd are sha256(salt || address and browser) under the salts of the two two-day spans the moment lies in: the one beginning on an even UTC day (counted from 1970-01-01) and the one beginning on an odd day.';
COMMENT ON TABLE net.visit_salts IS 'The salt of each two-day span (UTC) that is not over: span is its first day. Deleted when the span is over.';

-- What was counted before, by days of UTC+8 and without times, stays as it was: the totals
-- include it, the days and hours do not. Its last open day is closed now (its visitors counted,
-- their hashes and the salt deleted), as nothing more is counted there.
UPDATE net.visit_days d SET salt = NULL,
       visitors = (SELECT count(*) FROM net.visit_visitors v WHERE v.day = d.day)
 WHERE d.visitors IS NULL;
DELETE FROM net.visit_visitors;
COMMENT ON TABLE net.visit_days IS 'Page views per day (UTC+8) counted before views were kept with their times (0008); no more are added. visitors is the number of different visitors of the day.';
COMMENT ON TABLE net.visit_pages IS 'Page views per day (UTC+8) and page counted before views were kept with their times (0008); no more are added.';
COMMENT ON TABLE net.visit_visitors IS 'Empty since 0008: once the visitors of an open day of visit_days.';
