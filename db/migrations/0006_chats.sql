-- Players writing to each other (internal/chat): one chat for each two players who have
-- written, its two members, and their messages. A member has read their chat up to read_seq.
-- A player may block another: neither can then write to the other. The site's own notices
-- are not here: each source keeps them (notifications), and the website shows them as a chat
-- of their own.
CREATE TABLE IF NOT EXISTS net.chats (
    id text NOT NULL PRIMARY KEY,
    -- The two members' ids, the smaller first, joined by ':': one chat for two players.
    pair text NOT NULL UNIQUE,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_message_at timestamp with time zone
);

CREATE TABLE IF NOT EXISTS net.chat_members (
    chat_id text NOT NULL REFERENCES net.chats(id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES net.users(id) ON DELETE CASCADE,
    read_seq bigint DEFAULT 0 NOT NULL CHECK (read_seq >= 0),
    PRIMARY KEY (chat_id, user_id)
);
CREATE INDEX IF NOT EXISTS chat_members_user ON net.chat_members (user_id);

CREATE TABLE IF NOT EXISTS net.chat_messages (
    seq bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id text NOT NULL REFERENCES net.chats(id) ON DELETE CASCADE,
    sender_id text NOT NULL REFERENCES net.users(id) ON DELETE CASCADE,
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    FOREIGN KEY (chat_id, sender_id) REFERENCES net.chat_members(chat_id, user_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS chat_messages_chat ON net.chat_messages (chat_id, seq);

CREATE TABLE IF NOT EXISTS net.chat_blocks (
    user_id text NOT NULL REFERENCES net.users(id) ON DELETE CASCADE,
    blocked_id text NOT NULL REFERENCES net.users(id) ON DELETE CASCADE,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (user_id, blocked_id),
    CHECK (user_id <> blocked_id)
);

COMMENT ON TABLE net.chats IS 'A conversation between two players (pair: their ids, smaller first, joined by '':'').';
COMMENT ON TABLE net.chat_members IS 'The two players of a chat, and the last message each has read (read_seq).';
COMMENT ON TABLE net.chat_messages IS 'What players wrote to each other, numbered in the order written (seq, over all chats).';
COMMENT ON TABLE net.chat_blocks IS 'user_id has blocked blocked_id: neither can write to the other.';
