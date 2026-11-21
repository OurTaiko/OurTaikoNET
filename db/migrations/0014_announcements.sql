-- The site's announcements (internal/profile/httpapi/announcements.go): what its administrator
-- has to tell every visitor, shown on the home page and listed at /announcements. A title and
-- a text of plain lines, in whatever language the administrator writes; nothing here is read as
-- HTML or Markdown.
CREATE TABLE IF NOT EXISTS net.announcements (
    id text PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 80 AND title = btrim(title)),
    -- Empty when the title says it all.
    body text NOT NULL DEFAULT '' CHECK (char_length(body) <= 4000),
    -- Pinned ones are shown before the others, however old.
    pinned boolean NOT NULL DEFAULT false,
    -- When it was first announced; editing it later does not move it.
    published_at timestamptz NOT NULL DEFAULT now(),
    changed_by text NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE net.announcements IS 'Announcements of the site''s administrator to every visitor, shown on the home page: pinned ones first, then the newest.';

-- The order they are shown in.
CREATE INDEX IF NOT EXISTS announcements_shown ON net.announcements (pinned DESC, published_at DESC, id);
