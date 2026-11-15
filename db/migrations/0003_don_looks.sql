-- Each player's 3D Don as the game draws it (OurTaikoPlay Chara3D), chosen on the website: a
-- whole costume, or a head and a body; the puchichara beside it (0: none); and the three colours
-- (body, face, rim; null: the seat's defaults). The numbers are the models' in the public
-- bucket's 3dchara/catalog.json, which the server checks a choice against. No row: the plain
-- Don (costume 0), as a guest has.
CREATE TABLE IF NOT EXISTS net.don_looks (
    user_id text NOT NULL PRIMARY KEY,
    costume boolean NOT NULL DEFAULT true,
    cos_id integer NOT NULL DEFAULT 0 CHECK (cos_id >= 0),
    head_id integer NOT NULL DEFAULT 0 CHECK (head_id >= 0),
    body_id integer NOT NULL DEFAULT 0 CHECK (body_id >= 0),
    puchi_id integer NOT NULL DEFAULT 0 CHECK (puchi_id >= 0),
    colors text[] CHECK (colors IS NULL OR (array_length(colors, 1) = 3
        AND colors[1] ~ '^#[0-9A-F]{6}$' AND colors[2] ~ '^#[0-9A-F]{6}$' AND colors[3] ~ '^#[0-9A-F]{6}$')),
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

COMMENT ON TABLE net.don_looks IS 'Each player''s 3D Don (costume or head and body, puchichara, colours), chosen on the website; no row: the plain Don.';
