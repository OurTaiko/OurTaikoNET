-- How much the website is visited (internal/visits): the pages shown each day, and how many
-- different visitors saw them. No cookie is set and no address is kept: a visitor is told from
-- another, within one day only, by a hash of their address and browser with that day's salt,
-- and once the day is over the hashes are counted into visit_days.visitors and deleted with
-- the salt. A day is a day of UTC+8.
CREATE TABLE IF NOT EXISTS net.visit_days (
    day date NOT NULL PRIMARY KEY,
    views bigint NOT NULL DEFAULT 0 CHECK (views >= 0),
    visitors bigint CHECK (visitors >= 0),
    salt bytea,
    CHECK ((visitors IS NULL) = (salt IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS net.visit_pages (
    day date NOT NULL,
    page text NOT NULL,
    views bigint NOT NULL DEFAULT 0 CHECK (views >= 0),
    PRIMARY KEY (day, page)
);

CREATE TABLE IF NOT EXISTS net.visit_visitors (
    day date NOT NULL,
    visitor bytea NOT NULL,
    PRIMARY KEY (day, visitor)
);

COMMENT ON TABLE net.visit_days IS 'Page views of the website per day (UTC+8). While the day is open, salt is set and its visitors are the rows of visit_visitors; once closed, visitors holds their number and salt is null.';
COMMENT ON TABLE net.visit_pages IS 'Page views per day and page, a page being an address with its ids replaced by :id (/fanmade/charts/:id).';
COMMENT ON TABLE net.visit_visitors IS 'The visitors of a day that is still open: sha256(the day''s salt || address and browser). Deleted when the day is closed.';
