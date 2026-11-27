-- The parts a title is made of become rows, and a title names them by foreign key.
--
-- Until now a title's band, effect and icon were names checked by their spelling alone
-- (band_item, 'band/<name>'), and which names there were was known only to the file the bucket
-- published (nameplate/lumen.json, written from OurTaikoPlay's baked set). Now:
--
--   title_items    every part, under a number of its own: its type (band, effect, icon), its
--                  name (unique within its type: there is a band and an effect called
--                  lumen-title-gold), who it is cut for (kind: a player's plate or the AI's,
--                  which has another shape), where it comes from (source: the arcade's set or
--                  made for this game), names in other languages, and its file's SHA-256, which
--                  is its name in the bucket (nameplate/items/<hash>.otanim).
--   title_presets  the looks a maker starts from: the arcade's titles and the ones added since,
--                  and the AI plate's own (kind). They are offered in the order they were added
--                  (their numbers).
--
-- A title, a request for its review and a preset name their parts by number (band_id,
-- effect_id, icon_id). A number says nothing of the part's type, so a trigger sees that a band
-- is a band (title_parts_fit).
--
-- Whether an effect is drawn over the whole plate or cut to the band was a list of effects in
-- lumen.json (unclipped). It is the maker's choice now, per title (effect_overflow); the titles
-- and requests there are keep what that list gave them.
--
-- band_item, effect_item and icon_item stay for now, unread: the release before this one is
-- still answering while this runs, and reads them. The next change drops them.
CREATE TABLE IF NOT EXISTS net.title_items (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type text NOT NULL CONSTRAINT title_items_type CHECK (type IN ('band', 'effect', 'icon')),
    name text NOT NULL CONSTRAINT title_items_name CHECK (name ~ '^[a-z0-9][a-z0-9_.-]{0,63}$'),
    kind text NOT NULL DEFAULT 'player' CONSTRAINT title_items_kind CHECK (kind IN ('player', 'ai')),
    source text NOT NULL DEFAULT 'custom' CONSTRAINT title_items_source CHECK (source IN ('arcade', 'custom')),
    name_translations jsonb NOT NULL DEFAULT '{}'
        CONSTRAINT title_items_name_translations CHECK (jsonb_typeof(name_translations) = 'object'),
    hash text NOT NULL CONSTRAINT title_items_hash CHECK (hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT title_items_type_name UNIQUE (type, name)
);

COMMENT ON TABLE net.title_items IS 'The parts titles are made of: bands, effects over them and icons on their end. A title, a request for its review and a preset name theirs by id. The file is nameplate/items/<hash>.otanim under the distribution root.';
COMMENT ON COLUMN net.title_items.type IS 'band, effect or icon. Never changes (title_items_stay).';
COMMENT ON COLUMN net.title_items.name IS 'The part''s name, unique within its type. With the type before it (band/<name>) it is how a title''s layers name the part. Never changes.';
COMMENT ON COLUMN net.title_items.kind IS 'player: cut for a player''s plate, what a title may be made of. ai: cut for the AI plate''s other shape, worn only by the AI presets.';
COMMENT ON COLUMN net.title_items.source IS 'arcade: from the arcade''s nameplate set. custom: made for this game.';
COMMENT ON COLUMN net.title_items.name_translations IS 'The part''s name by language, for where parts are picked. Empty: shown by its name.';
COMMENT ON COLUMN net.title_items.hash IS 'SHA-256 of the part''s .otanim file, and its name in the bucket.';

-- The parts of the set published when this was written, numbered type by type in the order the
-- presets first wear them: the order they are offered in.
INSERT INTO net.title_items (type, name, kind, source, hash) VALUES
    ('band', 'lumen-title-wood', 'player', 'arcade', 'bb28f21f6ff8b66c1656d442d15471444949c652e90d18348b62fa0ad5b954a2'),
    ('band', 'lumen-title-rainbow', 'player', 'arcade', '08b71a4a0c18426f9342d870fa5d499b30a97f792bdfa12b9046da601b7bfb7b'),
    ('band', 'lumen-title-gold', 'player', 'arcade', '48b68a0befe98d45043453b8f3bd08e1451c33c262f8b09d82b8cd2b1ef4937c'),
    ('band', 'lumen-title-purple', 'player', 'arcade', '925b287ae2d01402c3f6e069dabb4d4aabb61303e144ce2ca2361cfa7d485798'),
    ('band', 'lumen-title-ai_1', 'player', 'arcade', 'bc040bf2a9de8c151e5bd6e4adc2d4d475447e669ae50a0a23a1e483f73882b5'),
    ('band', 'lumen-title-ai_2', 'player', 'arcade', '72b569a091703606be49e8dbee78a2bdcb14f48b8757a6eaa3abbe8311b7d2e1'),
    ('band', 'lumen-title-ai_3', 'player', 'arcade', 'b40341c27b7ddab6311ecb979582200547f7f9943ec026299b13810000df620a'),
    ('band', 'lumen-title-ai_4', 'player', 'arcade', '258ff91c4b85dce7c48754de3ef7a97480a6b05a2c86ddd42795e37bc92928f0'),
    ('band', 'lumen-title-onp_1', 'player', 'arcade', 'd05304e2367e9646a64c37d3339024e10469758becef09020805950a6719e5d1'),
    ('band', 'lumen-title-toho_y22', 'player', 'arcade', 'bec588eeff6f4f460e6a68885e222e4b9de0d4bb38ee8a13b5e66543ed461274'),
    ('band', 'lumen-title-aprilfool', 'player', 'arcade', '4e788a466ae39e58fa56df3409afa79be1a5e3effec02a6869f1fe8e86eca337'),
    ('band', 'lumen-title-aprilfool_6', 'player', 'arcade', '0d0830c2e99f648904111ea34124152af2371e5abe546b4fa05ae96f6b3ddc74'),
    ('band', 'lumen-title-million_y23_1', 'player', 'arcade', '9bb19d0faecfd88e3179ea8384f519fcda45491f0b5bf39ad749306f19666105'),
    ('band', 'lumen-title-million_y23_2', 'player', 'arcade', 'b0c7e8c427914e4e2af474eaec2205d31ef2c60fa0d86e62fa2bfea9f1420ef9'),
    ('band', 'lumen-title-million_y23_3', 'player', 'arcade', 'f4db93674df708fb66d2aba6aafa22b48a0cfb5bab49004dd682fc399aae6034'),
    ('band', 'lumen-title-shiny_y23_1', 'player', 'arcade', '263e78d034a0fe4f0462f3dfece2632b076e3371a728e57c9324e479023cdfa8'),
    ('band', 'lumen-title-shiny_y23_2', 'player', 'arcade', 'abd396f7d84797affd27cd78dc098795b7f674db5f6501936878262f663a35ea'),
    ('band', 'lumen-title-shiny_y23_3', 'player', 'arcade', '2b6eb6323dfe03757a61e08e6a6d89f50c5935cba083229aa5fadb082ae84263'),
    ('band', 'lumen-title-toho_y23', 'player', 'arcade', '69e6443b91377a5cffd6edc083e874a2dfc7571eec7ab0097301927c460e99b3'),
    ('band', 'lumen-title-sekaitaikai_01', 'player', 'arcade', '4bb19418dd6da0b87638f51bca9bc69ebb8e97504fea9c3446a74646017006e9'),
    ('band', 'lumen-title-sekaitaikai_02', 'player', 'arcade', '0f5c4be1044e13f55a8989b885f9fcc6e6e7f5aff4479287e734f204bc2734e4'),
    ('band', 'lumen-title-sekaitaikai_03', 'player', 'arcade', 'b917cf02d306b0cb99ab1f1ed4047b4a4e2d455a412ca96783ba0a96fd53252a'),
    ('band', 'lumen-title-sekaitaikai_04', 'player', 'arcade', '1d5ef2f14349393115a746a0c4364f0a29dfc591e19da1ee49f39389943637dd'),
    ('band', 'lumen-title-sekaitaikai', 'player', 'arcade', '13c9d9eea240a7ff2a8caf2ba651f7dde2d910be9f38d9d35a73f32d8c27b3a6'),
    ('band', 'lumen-title-aprilfool2024', 'player', 'arcade', '1e947f65012c110876dc4c5a0f16dbe5e7641921baae6a46e8025f011a48373a'),
    ('band', 'lumen-title-dani_red_01', 'player', 'arcade', '2bad29dd3c3ca4662c2f420ed4c38651eff53ba52052224af2af73074137065a'),
    ('band', 'lumen-title-dani_red_02', 'player', 'arcade', '4fa1f084e8c18e2b277ad8f7d14fbf242580bcb490602183d21a650209810d69'),
    ('band', 'lumen-title-crown_silver', 'player', 'arcade', '0f2a18f779a8609a392d05cc64801978d9b78aa6ec5399f04f9ec0316bc063c2'),
    ('band', 'lumen-title-crown_gold', 'player', 'arcade', '2da543ad2690c4e363ad674c9e481ac37a1bf4f10b27374d014c30820316f0b0'),
    ('band', 'lumen-title-crown_niji', 'player', 'arcade', '1d9d2113c3ee4ae941a1c9f046aba0409403b179c1bdc26bbfffddd5fd0dbdf9'),
    ('band', 'lumen-title-onp_y24', 'player', 'arcade', 'e614feb83a8251a427f86593740a27dde985e756faac880dc88fc2e5159c548b'),
    ('band', 'lumen-title-toho_y24', 'player', 'arcade', 'f0b745ca7ba294011dd2cedcc84dea3ac874f2cfca326494f521fccd46e5eeb1'),
    ('band', 'lumen-title-aprilfool2025', 'player', 'arcade', '22b9d5dd5767ad5b50ca9fcb186379d8b64414abe713428450b0b9e49681b3ec'),
    ('band', 'lumen-title-karapichi2025', 'player', 'arcade', '3d107f52ac87b7a13b388a7a88e78c7d255a61fc72c3f1d872d2d30934119c96'),
    ('band', 'lumen-title-wec', 'player', 'arcade', 'b9c2172ae863c35994468c3eeb831be70264ff68c4f9b2dd60df8a4fc2580159'),
    ('band', 'lumen-title-wec_05', 'player', 'arcade', 'fc398fc847712b197e72c2cdc747fc639ca9bb16581466b89bb1ec29217ec931'),
    ('band', 'lumen-title-wec_06', 'player', 'arcade', '62e56e1d737c813f8a827d94bf3351d27dd6786c8386d98a0132b0cf92807b94'),
    ('band', 'lumen-title-toho_y25', 'player', 'arcade', 'fab465213e237bf6fa6d6d77b53ca26980203c5265e106ced03b17463d8e66f3'),
    ('band', 'lumen-title-twso25', 'player', 'arcade', 'a959a82fa59f928cc599ea1a0bdf79164b0cece45aed28c332f2670d951afada'),
    ('band', 'lumen-title-twso25_5', 'player', 'arcade', 'a69734075b04119d71d43c598035faaa667d3b57df3f4bc72255d1a668f65c69'),
    ('band', 'lumen-title-twso25_6', 'player', 'arcade', '1cd5286e3dc6dd90bdeeda227b70223a950c484e86d7e4394e6a8679acd38695'),
    ('band', 'lumen-title-twso25_7', 'player', 'arcade', 'f82b8a6080e37a191bf21b60d30fed2d4fe1811a7d1d690d160873ca5d8464d1'),
    ('band', 'lumen-title-twso25_8', 'player', 'arcade', 'af851290fe0bfd6a958660eb8b8b77038b8676cff1dee6429781618112cc8eec'),
    ('band', 'jojo-dio-mansion', 'player', 'custom', '24739d07d5c3b38a1bbb95c15f5c0121798dd782dcf4b5a629b2a8a3c3853cd3'),
    ('band', 'lumen-ai-title-wood', 'ai', 'arcade', 'fc86e67a7fd0be0a4297fcd4e606e5c5dec58015d2c123363cd7a93834cc39e3'),
    ('band', 'lumen-ai-title-rainbow', 'ai', 'arcade', 'fb6a0525e65714ceae3653438f3c9033990639ef58ca4cfaf505ffac40e42abd'),
    ('band', 'lumen-ai-title-gold', 'ai', 'arcade', 'f443739bd4fdd3a3380fcd0bcad087ac3b42491354bfc50ff185686875bb8cb2'),
    ('band', 'lumen-ai-title-purple', 'ai', 'arcade', '2440e9dabfe47ad1256b80667297438e99374ac70ef0e1870a7e56fa5eaee10a'),
    ('band', 'lumen-ai-title-ai_1', 'ai', 'arcade', '17d60bd580802d3121c13d497905b20f1e5a8137a1b606a7418805ba278beaed'),
    ('band', 'lumen-ai-title-ai_2', 'ai', 'arcade', 'bfa253e1816dde35eb24de6d123dff0beb210835a77ba1b86a4170d618b774d7'),
    ('band', 'lumen-ai-title-ai_3', 'ai', 'arcade', 'c747729e609dad6340c13c9ca1ced21b01548948a12f2d6078224bdb0151ac71'),
    ('band', 'lumen-ai-title-ai_4', 'ai', 'arcade', 'bd17c977959f4d02314a1d15d2fcf246b38b86deefe4ca9c2efcaa1c43c61bf9'),
    ('effect', 'lumen-title-gold', 'player', 'arcade', '3d2f14d886e3746d6bfb7672f6ae4d7a28f66f404c6077bc471102f87e449f7b'),
    ('effect', 'lumen-title-purple', 'player', 'arcade', '3aced16238d67a33fffb83a28b1223f03b6c382b2c3962f9d83d81192ccdc820'),
    ('effect', 'lumen-title-ai_2', 'player', 'arcade', '61768e2db8bc59fcb8304deea69ee56f4af43928cf81466f95b29d74d21140f8'),
    ('effect', 'lumen-title-ai_3', 'player', 'arcade', 'b5f93b115ee75a6bf7c07540dd5ebfafd7cd567cdfb05ddc7888076678b227fa'),
    ('effect', 'lumen-title-ai_4', 'player', 'arcade', '4e2ceeb2f4fde58a0258a043af8f9fb02617ab84c189eecbac614ff36e2dc5ab'),
    ('effect', 'lumen-title-sekaitaikai_03', 'player', 'arcade', '2ad9e74554e9d35a6ca213a6f3bbda48b672fbbc2f07714a3afd304eb2f0d840'),
    ('effect', 'lumen-title-sekaitaikai', 'player', 'arcade', '2bd17e47b9c85fed61de8f553a2962d2d635f7c708bc5371bab0d1f3e506aee7'),
    ('effect', 'lumen-title-sekaitaikai_06', 'player', 'arcade', '86e0f79b3dc60a1f4fe0797712ffa621f11a4898dedc262a18d7e56ac848b030'),
    ('effect', 'lumen-title-sekaitaikai_07', 'player', 'arcade', '5a970d205ed3a9bed26c98f07ad59eed8af9ccd7ffcd233b44840b90f4cab641'),
    ('effect', 'lumen-title-sekaitaikai_08', 'player', 'arcade', '717828861f5acdd43388f36f7d5718c1d0f0e16c10005fa2e5abaf85279d4f0e'),
    ('effect', 'lumen-title-aprilfool2024', 'player', 'arcade', 'c8a7cbbd95f892ced7c26bc1ee0487c811d970fcb4f101fb2388ed5c4a8486e6'),
    ('effect', 'lumen-title-dani_red_01', 'player', 'arcade', '7f26aa4e71aa9758cd6b263c758b2e0d7aa8711bd439e44f58bd486dd94342a6'),
    ('effect', 'lumen-title-dani_red_02', 'player', 'arcade', 'b29b806b175ad466148a57fbddd48c85100eed07fde0d59c4eba54af7038d471'),
    ('effect', 'lumen-title-dani_red_03', 'player', 'arcade', '250b959b5aed731d2bd8280ea670a99c7df8e308ac112e4812646970490a30eb'),
    ('effect', 'lumen-title-dani_gold_01', 'player', 'arcade', 'f4488b20e043ce6f328c7428dc67e0561ef9936dea9d80cc5382db15a75773d3'),
    ('effect', 'lumen-title-twso25_1', 'player', 'arcade', 'f16c2e7f0c613a4d206dc5dc74f3045ef05dfc7f592c2de0fecf438d62618219'),
    ('effect', 'lumen-title-twso25_2', 'player', 'arcade', 'e27d4e621d14a8a9229236691827b4fbeaa87966746dcf2d6ca1befb474a2375'),
    ('effect', 'lumen-title-twso25_3', 'player', 'arcade', '0b032d842eeefd7c5ae3b4295455ff2ee9499998f93ab10a6912b7992e40db6a'),
    ('effect', 'lumen-title-twso25_4', 'player', 'arcade', '488746dfbb24c2cb669f794709f8e2f98809ecd700776a4919c1b58de558439e'),
    ('effect', 'lumen-title-twso25_5', 'player', 'arcade', '26199663f63ac97ffac48afc90d5afb93fe4c03c56db9e81199857d2c84b0cf7'),
    ('effect', 'lumen-title-twso25_6', 'player', 'arcade', 'b3e471229a138d3fe4ddfbac9962efcafbdafc43c2329919ed49b34f188b9ef6'),
    ('effect', 'lumen-title-twso25_r', 'player', 'arcade', '949612c25c7e49a23214d50aa5b76e7bf1726eb848484a4f5d49ee55f851fea3'),
    ('effect', 'jojo-overdrive', 'player', 'custom', '0b42df9f1bcc84b488fb37cd39ce6c3d109e1199e5def71a1a7e91bfea9793e5'),
    ('effect', 'lumen-ai-title-gold', 'ai', 'arcade', '0165271874cf60539e158381cc07c3c85e53201305f6b303c1d720e5f3a529ac'),
    ('effect', 'lumen-ai-title-purple', 'ai', 'arcade', '3d72d913a3ce59509efe25f78df230290fec996940103bc5d4a9f76259ad76aa'),
    ('effect', 'lumen-ai-title-ai_2', 'ai', 'arcade', '3e7c855c9498a7452f2d4d92954f8befa06fbd822d2c83cf7a6c23e8f2676174'),
    ('effect', 'lumen-ai-title-ai_3', 'ai', 'arcade', 'f87b139e64cf593fccfe4735753e63ab726a18e4e0cf0dc2686595d9204ab1bf'),
    ('effect', 'lumen-ai-title-ai_4', 'ai', 'arcade', '98be5af3c84d629459ae9d278125990e108d21e29281a698058a62aad1b3956b'),
    ('icon', 'lumen-title-toho_y22_qr', 'player', 'arcade', '4d61c1efc1ea7e60ed0cd7e2349a0bc9f74725753af7805edf850be0c99c0002'),
    ('icon', 'lumen-title-toho_y22_2', 'player', 'arcade', 'ec64ba4f739ce54353070dbd1a4d50fadae121adc619f456f49cdb1736297373'),
    ('icon', 'lumen-title-toho_y22_3', 'player', 'arcade', '192688cf765d8c96acf938cdb53779c1c925397cb19e71afdde505c8e432c922'),
    ('icon', 'lumen-title-toho_y22_4', 'player', 'arcade', '28850991397d0a68d3bf9a7cc5851a167388a5cfc74caa5650320b82b5e7deec'),
    ('icon', 'lumen-title-toho_y22_5', 'player', 'arcade', 'c3b8e95fcbef4272f9776768e4f331b2b72cbd93abeb7d574f80a5788bee1d3b'),
    ('icon', 'lumen-title-aprilfool_1', 'player', 'arcade', 'b7d63af29f1a53622537e83d315e249bd99e325c8178e62208574f934795a294'),
    ('icon', 'lumen-title-aprilfool_2', 'player', 'arcade', '7c79cb1ce57722fc69dd3d6685b117e3b96d1574683b203fde334d21f5d737bc'),
    ('icon', 'lumen-title-aprilfool_3', 'player', 'arcade', '50e634f4783190ba63074815c655ad859ce2d34f49475e9c7f9ccba8d96ad8ac'),
    ('icon', 'lumen-title-aprilfool_4', 'player', 'arcade', '7c752c17332c091abcb053583d593be03fba78ce8a2cb3e7022b30f29365d72c'),
    ('icon', 'lumen-title-aprilfool_5', 'player', 'arcade', '2a3f4721020f4f0647a84886c472d023ce38c366372b2f938f7dc94129e386c0'),
    ('icon', 'lumen-title-aprilfool_6', 'player', 'arcade', 'd78c020990cc37dd20693a3f79474a36c81fd14d49ab1b6e267c8bd435cf3787'),
    ('icon', 'lumen-title-million_y23_1', 'player', 'arcade', '3679b72056f4cdcd0e9da9254e2c17404517e0ed0a1c2f39eeaa74827e482780'),
    ('icon', 'lumen-title-million_y23_2', 'player', 'arcade', 'ecbda9528004eed32b469432da8a0ac7fb9df80a0c7c22aa504c06cffe89c883'),
    ('icon', 'lumen-title-million_y23_3', 'player', 'arcade', '9976c966a5a931dbd91dfff634e664798250b13c27f0553324f4444911f38b59'),
    ('icon', 'lumen-title-shiny_y23_1', 'player', 'arcade', '837012f6108100249159a41d8c39752467955f6afb18f52fa990da183a550207'),
    ('icon', 'lumen-title-shiny_y23_2', 'player', 'arcade', '19703d975449b413f6abe5715d15c030647a6380e885be787f97088e76710ec5'),
    ('icon', 'lumen-title-shiny_y23_3', 'player', 'arcade', 'd8a7c536aa9e27b7683d739e926feabb5131938c7f26ed309b8de02fbb1d70e8'),
    ('icon', 'lumen-title-toho_y23_1', 'player', 'arcade', '0e40feddd4705e6b2bad481ad844f38b7f277c2def97452cf2089c31f61b9053'),
    ('icon', 'lumen-title-toho_y23_2', 'player', 'arcade', '5abc58e4f858e7a7a2535665347c895728181aa7fd643ffeaa270d60b1cc9e7d'),
    ('icon', 'lumen-title-toho_y23_3', 'player', 'arcade', 'ccb105f3d2a368f0421edd17d00919e7adbd84258659a4b677cf882fd39244ae'),
    ('icon', 'lumen-title-toho_y23_4', 'player', 'arcade', 'f87c5216aee605df2ab7028c6d0cf5e4a161a61e8563c2634eb86403d077acf4'),
    ('icon', 'lumen-title-toho_y23_5', 'player', 'arcade', '703eea911a722c413ee4ce9a2756bde4eb082ba3a26f1cd61bb1f4a362002312'),
    ('icon', 'lumen-title-sekaitaikai_01', 'player', 'arcade', '8a33852a3482c43c1d0150fa223f5d10dc267a0f2c58c6f7d629f6a44c2f162b'),
    ('icon', 'lumen-title-sekaitaikai_02', 'player', 'arcade', '559b3a25090a41dd03e19603a84c2e0ed8403ee7da267e35c974747d3b325b12'),
    ('icon', 'lumen-title-sekaitaikai_03', 'player', 'arcade', '2823e671b772795779645407125f9ad8c5ea863a3ba62b9a7ae7195920a28bd9'),
    ('icon', 'lumen-title-sekaitaikai_04', 'player', 'arcade', 'a6e84acf9f61ae20fafbbeb6198f683e5a53cc4959674679fe38609fd21aed40'),
    ('icon', 'lumen-title-sekaitaikai_05', 'player', 'arcade', '7d4cb489aa836183133eead8dad6ac8697476b32c943d3192a0bd88a0ae06193'),
    ('icon', 'lumen-title-sekaitaikai_06', 'player', 'arcade', '0b170fc3a213b28d6c1c2cae0c1fc4fe965289a73c0639302ca71f0db47dc218'),
    ('icon', 'lumen-title-sekaitaikai_07', 'player', 'arcade', '8bfaa9e90d72f07e8d2e9051c794f3dc677eba089b119bf816be8a1599faebe9'),
    ('icon', 'lumen-title-sekaitaikai_08', 'player', 'arcade', '438674ec7d39ced87b7944e290142b4ee31bb0c06aab185b3263fdc5e5fe4e35'),
    ('icon', 'lumen-title-dani_red_01', 'player', 'arcade', '661bbd0471ef85093fbe71f9bd33694ee021dc6c5aa6cb94eeaf1cadec2dbef7'),
    ('icon', 'lumen-title-dani_red_02', 'player', 'arcade', '31f1af45ddbb00190493593a0dbdf503bf786f1cd8cc9eee459bf32f4174b685'),
    ('icon', 'lumen-title-dani_red_03', 'player', 'arcade', '2fe35f134b6e22e85bd93cc650bca5d2bf3e08991446dc55a2caa4dbe58a3103'),
    ('icon', 'lumen-title-dani_gold_01', 'player', 'arcade', '4db4506490ea3b9732c37a3c82835d9fdcdabd38e090290d6cbba4034c7450b3'),
    ('icon', 'lumen-title-dani_gold_02', 'player', 'arcade', '049d4809d498d7a6126970aa0500cead8bd9ff9c97bfc250d904dc4feaed8279'),
    ('icon', 'lumen-title-dani_gold_03', 'player', 'arcade', 'ea7f3f12473c22c586cc5d3e1ea45233dfcdb38b3275b0d90aef048adcb514e7'),
    ('icon', 'lumen-title-crown_silver', 'player', 'arcade', '0d660411016201cf57249905bef2b718d7c606d2f3fdfbb1dcbfbf616cabc0ac'),
    ('icon', 'lumen-title-crown_gold', 'player', 'arcade', '9167e13bdf0fdb21e69c38bdd355858954843ad4d87eccca64a91c0de532ff76'),
    ('icon', 'lumen-title-crown_niji', 'player', 'arcade', '8916653293f9e5454c49750aacd83d93c5f30f1ee2f1429b3565c94cfe7c2d8a'),
    ('icon', 'lumen-title-toho_y24_1', 'player', 'arcade', 'b60636869813699ee14833f9a250402b4c33464e47434859d5038bfe75b42b08'),
    ('icon', 'lumen-title-toho_y24_2', 'player', 'arcade', 'a7e955d96938b927589213660296db9eb9f4e1f51107b57789cc64cddf488027'),
    ('icon', 'lumen-title-toho_y24_3', 'player', 'arcade', '7aecc4a4d366e6ea09c0d9c0d6accbf0b49c70db894cc503bcd1c0d7532e7ea3'),
    ('icon', 'lumen-title-toho_y24_4', 'player', 'arcade', '4d2aebbf9b5e128b68ad766af55089ffcd3dc45d1110a50411ed129e4a98b299'),
    ('icon', 'lumen-title-toho_y24_5', 'player', 'arcade', 'e709eaa3a24ed91f2c9d205b6bd0942d9e5c99ed89e5f796a544cb734a404fdd'),
    ('icon', 'lumen-title-aprilfool2025_01', 'player', 'arcade', '0816842fa6b38e28ce3801fb3b8a426c7da74dde0ae6817b92f77c7ca3b13783'),
    ('icon', 'lumen-title-aprilfool2025_02', 'player', 'arcade', '6f5691ca9253f9061ef24e23f8a1e5099e59cec2f64aaf689add981632f59595'),
    ('icon', 'lumen-title-aprilfool2025_03', 'player', 'arcade', 'd5c23e03a78432932e7889a166421ddf9d06b2f45f084e478e4c5992db04f0fb'),
    ('icon', 'lumen-title-crown_silver_ura', 'player', 'arcade', '74ae8dc55365d8c2990c714ef725ce6ca7e6fdcc78c3d6f1d23eaffa453c6c0f'),
    ('icon', 'lumen-title-crown_gold_ura', 'player', 'arcade', '86d8aa5aa16c1e5e36eba1551760b5a32b9a9d27f2bdb8d22d36f20d037971a1'),
    ('icon', 'lumen-title-crown_niji_ura', 'player', 'arcade', '4c5d42be84bf14e6da28652e5832cf38e6d8ad44f3aaec3ab65ef7d1c2e3da1b'),
    ('icon', 'lumen-title-wec', 'player', 'arcade', '7da5a75c286117130ae2df28c17f6bf8363fd9196962028c307a0e677ed652b6'),
    ('icon', 'lumen-title-wec-2', 'player', 'arcade', '3f8358ae31d187f14ce3eee3e43cf1fd8027acfce05124d5ee1e36156982bc3f'),
    ('icon', 'lumen-title-wec_06', 'player', 'arcade', '391fcc99efb2373209ddc9ea93facd84a11fa78a6dc019cecad05377acb5c1dd'),
    ('icon', 'lumen-title-toho_y25_1', 'player', 'arcade', '822dca8f438ef74a13790c6e1dcc40233a23fc042ad086e8569f74f5592c29f2'),
    ('icon', 'lumen-title-toho_y25_2', 'player', 'arcade', 'ad9d0f677d24c7123bbb6779a69aa439b9a35cdad811b34117ab2241895957ac'),
    ('icon', 'lumen-title-toho_y25_3', 'player', 'arcade', '073dbfcff5dee19a923e13ce11a9642cd827f946a18ef2fbb3290fb1b4f32f53'),
    ('icon', 'lumen-title-toho_y25_4', 'player', 'arcade', 'bfdedaaae9dc1f270081ea6c9eb214bcd20ff38cc25fe43daf3c70923deb8f92'),
    ('icon', 'lumen-title-toho_y25_5', 'player', 'arcade', 'ae70d87663bd00b9104f809ea01b5edfdefd7605ae7ffaf300ca82c2873bb3b5'),
    ('icon', 'lumen-title-twso25_1', 'player', 'arcade', '0e27353245eaddfd739030fbd6e52e62b7a0025868016182238688beb1f1046a'),
    ('icon', 'lumen-title-twso25_2', 'player', 'arcade', '34cb7e325273b65fbac68126cb5eb515ce361a2a806e2671eb601ee841e6d8c8'),
    ('icon', 'lumen-title-twso25_3', 'player', 'arcade', '35219f61cbdffb05fd128f758bec933c2fde7780e9601cf9bf9c97fa3f889813'),
    ('icon', 'lumen-title-twso25_4', 'player', 'arcade', '1d235989018682cd13f7049f9e82f688de3d9f5ceed605d8fe407d8c7626a0c3'),
    ('icon', 'lumen-title-twso25_5', 'player', 'arcade', '20d8160575a607ab3fd4bd3e5881ecb33a5744cf299c87ea07dac5e640f8cc02'),
    ('icon', 'lumen-title-twso25_6', 'player', 'arcade', 'c9736a895c79455b0bd06c13e9b77ac228385152396cb36101b3e4fbd6f1e2cc'),
    ('icon', 'lumen-title-twso25', 'player', 'arcade', '0a8621ec4da3fedc9ec4578dcf639121da8dfc3ce20e9548bd1f5d522fef5e47'),
    ('icon', 'lumen-title-twso25_r_1', 'player', 'arcade', '5d3235de6d39f9276c192c058e8b1b377fd7aaae3b4adbdec4dcfd24d7bc05fb'),
    ('icon', 'lumen-title-twso25_r_2', 'player', 'arcade', '7f91a7a0976574f113eaa911263829157657084628391de59edcca6bdecafb56'),
    ('icon', 'lumen-title-twso25_r_3', 'player', 'arcade', '99a2d04c867b7496d96a7b5cd3dc0c2095d646d91d349758f275ced1684b65f9'),
    ('icon', 'lumen-title-twso25_r_4', 'player', 'arcade', 'ef7b12bd72043295a9fd9fb1287f6371ca651cc6efb9ca6e5617f5c2185e4642'),
    ('icon', 'jojo-jonathan-portrait', 'player', 'custom', '462cf6c32417bfd0281e05a02ab3692240d4d56a66503450f016c8b400f2df60')
ON CONFLICT (type, name) DO NOTHING;

-- A part keeps its type and its name: titles and presets are made of it as what it is, and its
-- name is how the API has named it to those who keep it. A new picture is a new hash on the same
-- row; another part is another row.
CREATE OR REPLACE FUNCTION net.title_item_stays() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  RAISE EXCEPTION 'the type and the name of a part do not change' USING ERRCODE = 'check_violation';
END $$;
DROP TRIGGER IF EXISTS title_items_stay ON net.title_items;
CREATE TRIGGER title_items_stay BEFORE UPDATE OF type, name ON net.title_items
    FOR EACH ROW WHEN (NEW.type IS DISTINCT FROM OLD.type OR NEW.name IS DISTINCT FROM OLD.name)
    EXECUTE FUNCTION net.title_item_stays();

-- The part of that type and name; none when there is no such part.
CREATE OR REPLACE FUNCTION net.title_item_id(item_type text, item_name text) RETURNS integer
    LANGUAGE sql STABLE SET search_path = net AS
$$ SELECT id FROM net.title_items WHERE type = item_type AND name = item_name $$;

-- A row's band is a band, its effect an effect and its icon an icon.
CREATE OR REPLACE FUNCTION net.title_parts_fit() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  IF (SELECT type FROM net.title_items WHERE id = NEW.band_id) IS DISTINCT FROM 'band'
     OR (NEW.effect_id IS NOT NULL AND (SELECT type FROM net.title_items WHERE id = NEW.effect_id) IS DISTINCT FROM 'effect')
     OR (NEW.icon_id IS NOT NULL AND (SELECT type FROM net.title_items WHERE id = NEW.icon_id) IS DISTINCT FROM 'icon') THEN
    RAISE EXCEPTION 'a part of % is of another type', TG_TABLE_NAME USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;

CREATE TABLE IF NOT EXISTS net.title_presets (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL CONSTRAINT title_presets_kind CHECK (kind IN ('player', 'ai')),
    name text NOT NULL CONSTRAINT title_presets_name CHECK (name ~ '^[a-z0-9][a-z0-9_.-]{0,63}$'),
    band_id integer NOT NULL CONSTRAINT title_presets_band REFERENCES net.title_items (id),
    effect_id integer CONSTRAINT title_presets_effect REFERENCES net.title_items (id),
    icon_id integer CONSTRAINT title_presets_icon REFERENCES net.title_items (id),
    effect_overflow boolean NOT NULL DEFAULT false,
    CONSTRAINT title_presets_kind_name UNIQUE (kind, name),
    CONSTRAINT title_presets_effect_overflow CHECK (effect_id IS NOT NULL OR NOT effect_overflow)
);
DROP TRIGGER IF EXISTS title_presets_parts_fit ON net.title_presets;
CREATE TRIGGER title_presets_parts_fit BEFORE INSERT OR UPDATE OF band_id, effect_id, icon_id ON net.title_presets
    FOR EACH ROW EXECUTE FUNCTION net.title_parts_fit();

COMMENT ON TABLE net.title_presets IS 'The looks a title''s maker starts from: a band, and perhaps an effect and an icon, under a name. The arcade''s titles and the ones added since (kind player), and the AI plate''s own of the same names (kind ai), offered in the order of their ids.';
COMMENT ON COLUMN net.title_presets.effect_overflow IS 'Whether the look draws its effect over the whole plate instead of cutting it to the band.';

-- The presets of that set, numbered in the order the arcade has them.
INSERT INTO net.title_presets (kind, name, band_id, effect_id, icon_id, effect_overflow)
SELECT v.kind, v.name, net.title_item_id('band', v.band), net.title_item_id('effect', v.effect),
       net.title_item_id('icon', v.icon), v.overflow
FROM (VALUES
    (1, 'player', 'wood', 'lumen-title-wood', NULL, NULL, false),
    (2, 'player', 'rainbow', 'lumen-title-rainbow', NULL, NULL, false),
    (3, 'player', 'gold', 'lumen-title-gold', 'lumen-title-gold', NULL, true),
    (4, 'player', 'purple', 'lumen-title-purple', 'lumen-title-purple', NULL, true),
    (5, 'player', 'ai_1', 'lumen-title-ai_1', NULL, NULL, false),
    (6, 'player', 'ai_2', 'lumen-title-ai_2', 'lumen-title-ai_2', NULL, false),
    (7, 'player', 'ai_3', 'lumen-title-ai_3', 'lumen-title-ai_3', NULL, false),
    (8, 'player', 'ai_4', 'lumen-title-ai_4', 'lumen-title-ai_4', NULL, false),
    (9, 'player', 'onp_1', 'lumen-title-onp_1', NULL, NULL, false),
    (10, 'player', 'toho_y22_qr', 'lumen-title-rainbow', NULL, 'lumen-title-toho_y22_qr', false),
    (11, 'player', 'toho_y22_1', 'lumen-title-toho_y22', NULL, 'lumen-title-toho_y22_qr', false),
    (12, 'player', 'toho_y22_2', 'lumen-title-toho_y22', NULL, 'lumen-title-toho_y22_2', false),
    (13, 'player', 'toho_y22_3', 'lumen-title-toho_y22', NULL, 'lumen-title-toho_y22_3', false),
    (14, 'player', 'toho_y22_4', 'lumen-title-toho_y22', NULL, 'lumen-title-toho_y22_4', false),
    (15, 'player', 'toho_y22_5', 'lumen-title-toho_y22', NULL, 'lumen-title-toho_y22_5', false),
    (16, 'player', 'aprilfool_1', 'lumen-title-aprilfool', NULL, 'lumen-title-aprilfool_1', false),
    (17, 'player', 'aprilfool_2', 'lumen-title-aprilfool', NULL, 'lumen-title-aprilfool_2', false),
    (18, 'player', 'aprilfool_3', 'lumen-title-aprilfool', NULL, 'lumen-title-aprilfool_3', false),
    (19, 'player', 'aprilfool_4', 'lumen-title-aprilfool', NULL, 'lumen-title-aprilfool_4', false),
    (20, 'player', 'aprilfool_5', 'lumen-title-aprilfool', NULL, 'lumen-title-aprilfool_5', false),
    (21, 'player', 'aprilfool_6', 'lumen-title-aprilfool_6', NULL, 'lumen-title-aprilfool_6', false),
    (22, 'player', 'million_y23_1', 'lumen-title-million_y23_1', NULL, 'lumen-title-million_y23_1', false),
    (23, 'player', 'million_y23_2', 'lumen-title-million_y23_2', NULL, 'lumen-title-million_y23_2', false),
    (24, 'player', 'million_y23_3', 'lumen-title-million_y23_3', NULL, 'lumen-title-million_y23_3', false),
    (25, 'player', 'shiny_y23_1', 'lumen-title-shiny_y23_1', NULL, 'lumen-title-shiny_y23_1', false),
    (26, 'player', 'shiny_y23_2', 'lumen-title-shiny_y23_2', NULL, 'lumen-title-shiny_y23_2', false),
    (27, 'player', 'shiny_y23_3', 'lumen-title-shiny_y23_3', NULL, 'lumen-title-shiny_y23_3', false),
    (28, 'player', 'toho_y23_1', 'lumen-title-toho_y23', NULL, 'lumen-title-toho_y23_1', false),
    (29, 'player', 'toho_y23_2', 'lumen-title-toho_y23', NULL, 'lumen-title-toho_y23_2', false),
    (30, 'player', 'toho_y23_3', 'lumen-title-toho_y23', NULL, 'lumen-title-toho_y23_3', false),
    (31, 'player', 'toho_y23_4', 'lumen-title-toho_y23', NULL, 'lumen-title-toho_y23_4', false),
    (32, 'player', 'toho_y23_5', 'lumen-title-toho_y23', NULL, 'lumen-title-toho_y23_5', false),
    (33, 'player', 'sekaitaikai_01', 'lumen-title-sekaitaikai_01', NULL, 'lumen-title-sekaitaikai_01', false),
    (34, 'player', 'sekaitaikai_02', 'lumen-title-sekaitaikai_02', NULL, 'lumen-title-sekaitaikai_02', false),
    (35, 'player', 'sekaitaikai_03', 'lumen-title-sekaitaikai_03', 'lumen-title-sekaitaikai_03', 'lumen-title-sekaitaikai_03', true),
    (36, 'player', 'sekaitaikai_04', 'lumen-title-sekaitaikai_04', 'lumen-title-sekaitaikai', 'lumen-title-sekaitaikai_04', true),
    (37, 'player', 'sekaitaikai_05', 'lumen-title-sekaitaikai', 'lumen-title-sekaitaikai', 'lumen-title-sekaitaikai_05', true),
    (38, 'player', 'sekaitaikai_06', 'lumen-title-sekaitaikai', 'lumen-title-sekaitaikai_06', 'lumen-title-sekaitaikai_06', true),
    (39, 'player', 'sekaitaikai_07', 'lumen-title-sekaitaikai', 'lumen-title-sekaitaikai_07', 'lumen-title-sekaitaikai_07', true),
    (40, 'player', 'sekaitaikai_08', 'lumen-title-sekaitaikai', 'lumen-title-sekaitaikai_08', 'lumen-title-sekaitaikai_08', true),
    (41, 'player', 'aprilfool2024', 'lumen-title-aprilfool2024', 'lumen-title-aprilfool2024', NULL, true),
    (42, 'player', 'dani_red_01', 'lumen-title-dani_red_01', 'lumen-title-dani_red_01', 'lumen-title-dani_red_01', false),
    (43, 'player', 'dani_red_02', 'lumen-title-dani_red_02', 'lumen-title-dani_red_02', 'lumen-title-dani_red_02', false),
    (44, 'player', 'dani_red_03', 'lumen-title-dani_red_02', 'lumen-title-dani_red_03', 'lumen-title-dani_red_03', false),
    (45, 'player', 'dani_gold_01', 'lumen-title-dani_red_02', 'lumen-title-dani_gold_01', 'lumen-title-dani_gold_01', false),
    (46, 'player', 'dani_gold_02', 'lumen-title-dani_red_02', 'lumen-title-dani_red_02', 'lumen-title-dani_gold_02', false),
    (47, 'player', 'dani_gold_03', 'lumen-title-dani_red_02', 'lumen-title-dani_red_03', 'lumen-title-dani_gold_03', false),
    (48, 'player', 'crown_silver', 'lumen-title-crown_silver', NULL, 'lumen-title-crown_silver', false),
    (49, 'player', 'crown_gold', 'lumen-title-crown_gold', NULL, 'lumen-title-crown_gold', false),
    (50, 'player', 'crown_niji', 'lumen-title-crown_niji', NULL, 'lumen-title-crown_niji', false),
    (51, 'player', 'onp_y24', 'lumen-title-onp_y24', NULL, NULL, false),
    (52, 'player', 'toho_y24_1', 'lumen-title-toho_y24', NULL, 'lumen-title-toho_y24_1', false),
    (53, 'player', 'toho_y24_2', 'lumen-title-toho_y24', NULL, 'lumen-title-toho_y24_2', false),
    (54, 'player', 'toho_y24_3', 'lumen-title-toho_y24', NULL, 'lumen-title-toho_y24_3', false),
    (55, 'player', 'toho_y24_4', 'lumen-title-toho_y24', NULL, 'lumen-title-toho_y24_4', false),
    (56, 'player', 'toho_y24_5', 'lumen-title-toho_y24', NULL, 'lumen-title-toho_y24_5', false),
    (57, 'player', 'aprilfool2025_01', 'lumen-title-aprilfool2025', NULL, 'lumen-title-aprilfool2025_01', false),
    (58, 'player', 'aprilfool2025_02', 'lumen-title-aprilfool2025', NULL, 'lumen-title-aprilfool2025_02', false),
    (59, 'player', 'aprilfool2025_03', 'lumen-title-aprilfool2025', NULL, 'lumen-title-aprilfool2025_03', false),
    (60, 'player', 'crown_silver_ura', 'lumen-title-crown_silver', NULL, 'lumen-title-crown_silver_ura', false),
    (61, 'player', 'crown_gold_ura', 'lumen-title-crown_gold', NULL, 'lumen-title-crown_gold_ura', false),
    (62, 'player', 'crown_niji_ura', 'lumen-title-crown_niji', NULL, 'lumen-title-crown_niji_ura', false),
    (63, 'player', 'karapichi2025', 'lumen-title-karapichi2025', NULL, NULL, false),
    (64, 'player', 'wec_01', 'lumen-title-wec', NULL, 'lumen-title-wec', false),
    (65, 'player', 'wec_02', 'lumen-title-wec', NULL, 'lumen-title-wec', false),
    (66, 'player', 'wec_03', 'lumen-title-wec', NULL, 'lumen-title-wec', false),
    (67, 'player', 'wec_04', 'lumen-title-wec', NULL, 'lumen-title-wec-2', false),
    (68, 'player', 'wec_05', 'lumen-title-wec_05', NULL, 'lumen-title-wec-2', false),
    (69, 'player', 'wec_06', 'lumen-title-wec_06', NULL, 'lumen-title-wec_06', false),
    (70, 'player', 'toho_y25_1', 'lumen-title-toho_y25', NULL, 'lumen-title-toho_y25_1', false),
    (71, 'player', 'toho_y25_2', 'lumen-title-toho_y25', NULL, 'lumen-title-toho_y25_2', false),
    (72, 'player', 'toho_y25_3', 'lumen-title-toho_y25', NULL, 'lumen-title-toho_y25_3', false),
    (73, 'player', 'toho_y25_4', 'lumen-title-toho_y25', NULL, 'lumen-title-toho_y25_4', false),
    (74, 'player', 'toho_y25_5', 'lumen-title-toho_y25', NULL, 'lumen-title-toho_y25_5', false),
    (75, 'player', 'twso25_1', 'lumen-title-twso25', 'lumen-title-twso25_1', 'lumen-title-twso25_1', true),
    (76, 'player', 'twso25_2', 'lumen-title-twso25', 'lumen-title-twso25_2', 'lumen-title-twso25_2', true),
    (77, 'player', 'twso25_3', 'lumen-title-twso25', 'lumen-title-twso25_3', 'lumen-title-twso25_3', true),
    (78, 'player', 'twso25_4', 'lumen-title-twso25', 'lumen-title-twso25_4', 'lumen-title-twso25_4', true),
    (79, 'player', 'twso25_5', 'lumen-title-twso25_5', 'lumen-title-twso25_5', 'lumen-title-twso25_5', true),
    (80, 'player', 'twso25_6', 'lumen-title-twso25_6', 'lumen-title-twso25_6', 'lumen-title-twso25_6', true),
    (81, 'player', 'twso25_7', 'lumen-title-twso25_7', NULL, 'lumen-title-twso25', false),
    (82, 'player', 'twso25_8', 'lumen-title-twso25_8', NULL, 'lumen-title-twso25', false),
    (83, 'player', 'twso25_r_1', 'lumen-title-twso25_6', 'lumen-title-twso25_r', 'lumen-title-twso25_r_1', true),
    (84, 'player', 'twso25_r_2', 'lumen-title-twso25_6', 'lumen-title-twso25_r', 'lumen-title-twso25_r_2', true),
    (85, 'player', 'twso25_r_3', 'lumen-title-twso25_6', 'lumen-title-twso25_r', 'lumen-title-twso25_r_3', true),
    (86, 'player', 'twso25_r_4', 'lumen-title-twso25_6', 'lumen-title-twso25_6', 'lumen-title-twso25_r_4', true),
    (87, 'player', 'twso25_r_5', 'lumen-title-twso25_6', 'lumen-title-twso25_6', 'lumen-title-twso25_6', true),
    (88, 'player', 'pjsekai', 'lumen-title-rainbow', NULL, NULL, false),
    (89, 'player', 'jojo_1', 'jojo-dio-mansion', 'jojo-overdrive', 'jojo-jonathan-portrait', true),
    (90, 'ai', 'wood', 'lumen-ai-title-wood', NULL, NULL, false),
    (91, 'ai', 'rainbow', 'lumen-ai-title-rainbow', NULL, NULL, false),
    (92, 'ai', 'gold', 'lumen-ai-title-gold', 'lumen-ai-title-gold', NULL, true),
    (93, 'ai', 'purple', 'lumen-ai-title-purple', 'lumen-ai-title-purple', NULL, true),
    (94, 'ai', 'ai_1', 'lumen-ai-title-ai_1', NULL, NULL, false),
    (95, 'ai', 'ai_2', 'lumen-ai-title-ai_2', 'lumen-ai-title-ai_2', NULL, false),
    (96, 'ai', 'ai_3', 'lumen-ai-title-ai_3', 'lumen-ai-title-ai_3', NULL, false),
    (97, 'ai', 'ai_4', 'lumen-ai-title-ai_4', 'lumen-ai-title-ai_4', NULL, true)
) AS v (n, kind, name, band, effect, icon, overflow)
ORDER BY v.n
ON CONFLICT (kind, name) DO NOTHING;

-- The effects lumen.json listed as unclipped: what the titles and requests there are were drawn
-- with until now.
CREATE TEMPORARY TABLE IF NOT EXISTS overflowing ON COMMIT DROP AS
    SELECT unnest(ARRAY[
        'jojo-overdrive', 'lumen-title-aprilfool2024', 'lumen-title-gold', 'lumen-title-purple',
        'lumen-title-sekaitaikai', 'lumen-title-sekaitaikai_03', 'lumen-title-sekaitaikai_06', 'lumen-title-sekaitaikai_07',
        'lumen-title-sekaitaikai_08', 'lumen-title-twso25_1', 'lumen-title-twso25_2', 'lumen-title-twso25_3',
        'lumen-title-twso25_4', 'lumen-title-twso25_5', 'lumen-title-twso25_6', 'lumen-title-twso25_r']) AS name;

-- Once: a second run must not put back what band_item says over what was chosen since.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                 WHERE table_schema = 'net' AND table_name = 'titles' AND column_name = 'band_id') THEN
    ALTER TABLE net.titles
        ADD COLUMN band_id integer NOT NULL DEFAULT net.title_item_id('band', 'lumen-title-wood')
            CONSTRAINT titles_band REFERENCES net.title_items (id),
        ADD COLUMN effect_id integer CONSTRAINT titles_effect REFERENCES net.title_items (id),
        ADD COLUMN icon_id integer CONSTRAINT titles_icon REFERENCES net.title_items (id),
        ADD COLUMN effect_overflow boolean NOT NULL DEFAULT false,
        ADD CONSTRAINT titles_effect_overflow CHECK (effect_id IS NOT NULL OR NOT effect_overflow);
    UPDATE net.titles t SET
        band_id = COALESCE((SELECT i.id FROM net.title_items i
                            WHERE i.type = 'band' AND i.kind = 'player' AND 'band/' || i.name = t.band_item),
                           net.title_item_id('band', 'lumen-title-wood')),
        effect_id = (SELECT i.id FROM net.title_items i
                     WHERE i.type = 'effect' AND i.kind = 'player' AND 'effect/' || i.name = t.effect_item),
        icon_id = (SELECT i.id FROM net.title_items i
                   WHERE i.type = 'icon' AND i.kind = 'player' AND 'icon/' || i.name = t.icon_item);
    UPDATE net.titles t SET effect_overflow = true
    WHERE (SELECT i.name FROM net.title_items i WHERE i.id = t.effect_id) IN (SELECT name FROM overflowing);
    CREATE TRIGGER titles_parts_fit BEFORE INSERT OR UPDATE OF band_id, effect_id, icon_id ON net.titles
        FOR EACH ROW EXECUTE FUNCTION net.title_parts_fit();
  END IF;
END $$;

COMMENT ON COLUMN net.titles.band_id IS 'The band a nameplate wears for this title (title_items, type band).';
COMMENT ON COLUMN net.titles.effect_id IS 'The effect drawn over the band (title_items, type effect), or none.';
COMMENT ON COLUMN net.titles.icon_id IS 'The icon on the band''s end (title_items, type icon), or none.';
COMMENT ON COLUMN net.titles.effect_overflow IS 'Whether the effect is drawn over the whole plate instead of being cut to the band. Its maker''s choice; false without an effect.';

-- Once: a second run must not put back what band_item says over what was chosen since.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                 WHERE table_schema = 'net' AND table_name = 'title_reviews' AND column_name = 'band_id') THEN
    ALTER TABLE net.title_reviews
        ADD COLUMN band_id integer NOT NULL DEFAULT net.title_item_id('band', 'lumen-title-wood')
            CONSTRAINT title_reviews_band REFERENCES net.title_items (id),
        ADD COLUMN effect_id integer CONSTRAINT title_reviews_effect REFERENCES net.title_items (id),
        ADD COLUMN icon_id integer CONSTRAINT title_reviews_icon REFERENCES net.title_items (id),
        ADD COLUMN effect_overflow boolean NOT NULL DEFAULT false,
        ADD CONSTRAINT title_reviews_effect_overflow CHECK (effect_id IS NOT NULL OR NOT effect_overflow);
    UPDATE net.title_reviews t SET
        band_id = COALESCE((SELECT i.id FROM net.title_items i
                            WHERE i.type = 'band' AND i.kind = 'player' AND 'band/' || i.name = t.band_item),
                           net.title_item_id('band', 'lumen-title-wood')),
        effect_id = (SELECT i.id FROM net.title_items i
                     WHERE i.type = 'effect' AND i.kind = 'player' AND 'effect/' || i.name = t.effect_item),
        icon_id = (SELECT i.id FROM net.title_items i
                   WHERE i.type = 'icon' AND i.kind = 'player' AND 'icon/' || i.name = t.icon_item);
    UPDATE net.title_reviews t SET effect_overflow = true
    WHERE (SELECT i.name FROM net.title_items i WHERE i.id = t.effect_id) IN (SELECT name FROM overflowing);
    CREATE TRIGGER title_reviews_parts_fit BEFORE INSERT OR UPDATE OF band_id, effect_id, icon_id ON net.title_reviews
        FOR EACH ROW EXECUTE FUNCTION net.title_parts_fit();
  END IF;
END $$;

COMMENT ON COLUMN net.title_reviews.band_id IS 'The band the title is asked to wear (title_items, type band).';
COMMENT ON COLUMN net.title_reviews.effect_id IS 'The effect drawn over the band (title_items, type effect), or none.';
COMMENT ON COLUMN net.title_reviews.icon_id IS 'The icon on the band''s end (title_items, type icon), or none.';
COMMENT ON COLUMN net.title_reviews.effect_overflow IS 'Whether the effect is drawn over the whole plate instead of being cut to the band. Its maker''s choice; false without an effect.';

-- While the old names are still there (0030 drops them, and this is run again on databases that
-- have none, or have them on titles alone, where 0022 run again puts them back): they are said to
-- be unread, and a request made by the release before, while this ran, has no band_item of its
-- own any more.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'net' AND table_name = 'titles' AND column_name = 'band_item') THEN
    COMMENT ON COLUMN net.titles.band_item IS 'Unread since title_items (band_id); kept for the release before, dropped by the next change.';
    COMMENT ON COLUMN net.titles.effect_item IS 'Unread since title_items (effect_id).';
    COMMENT ON COLUMN net.titles.icon_item IS 'Unread since title_items (icon_id).';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'net' AND table_name = 'title_reviews' AND column_name = 'band_item') THEN
    ALTER TABLE net.title_reviews ALTER COLUMN band_item SET DEFAULT 'band/lumen-title-wood';
  END IF;
END $$;

-- A title given is told as it looks now, with where its pictures are. One given before and not
-- seen yet is told the same way: nothing reads a title without them any more.
UPDATE net.rewards r SET data = jsonb_set(r.data, '{title,layers}', (SELECT jsonb_build_object(
      'band', (SELECT 'band/' || name FROM net.title_items WHERE id = t.band_id),
      'effect', (SELECT 'effect/' || name FROM net.title_items WHERE id = t.effect_id),
      'icon', (SELECT 'icon/' || name FROM net.title_items WHERE id = t.icon_id),
      'effectOverflow', t.effect_overflow,
      'hashes', jsonb_build_object(
        'band', (SELECT hash FROM net.title_items WHERE id = t.band_id),
        'effect', (SELECT hash FROM net.title_items WHERE id = t.effect_id),
        'icon', (SELECT hash FROM net.title_items WHERE id = t.icon_id)))
    FROM net.titles t WHERE t.id = r.data #>> '{title,id}'))
WHERE r.kind = 'title' AND r.seen_at IS NULL
  AND EXISTS (SELECT 1 FROM net.titles t WHERE t.id = r.data #>> '{title,id}');

CREATE OR REPLACE FUNCTION net.reward_title() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  INSERT INTO rewards(user_id, kind, data)
  SELECT NEW.user_id, 'title', jsonb_build_object(
    'title', jsonb_build_object('id', t.id, 'name', t.name, 'nameTranslations', t.name_translations,
      'description', t.description, 'obtain', t.obtain, 'style', t.style,
      'visibility', t.visibility, 'ownerId', t.owner_id,
      'layers', jsonb_build_object(
      'band', (SELECT 'band/' || name FROM net.title_items WHERE id = t.band_id),
      'effect', (SELECT 'effect/' || name FROM net.title_items WHERE id = t.effect_id),
      'icon', (SELECT 'icon/' || name FROM net.title_items WHERE id = t.icon_id),
      'effectOverflow', t.effect_overflow,
      'hashes', jsonb_build_object(
        'band', (SELECT hash FROM net.title_items WHERE id = t.band_id),
        'effect', (SELECT hash FROM net.title_items WHERE id = t.effect_id),
        'icon', (SELECT hash FROM net.title_items WHERE id = t.icon_id)))),
    'service', a.service, 'courseTitle', a.course_title)
  FROM titles t LEFT JOIN awards a ON a.id = NEW.award_id
  WHERE t.id = NEW.title_id AND t.owner_id IS DISTINCT FROM NEW.user_id;
  RETURN NULL;
END $$;
