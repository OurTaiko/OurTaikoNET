package httpapi

// The song sources. A server serves one of them (Config.Source).

// Fanmade is the source of songs players make and upload themselves.
var Fanmade = &Source{
	Folder:        "fanmade/",
	SessionCookie: "ourtaiko_session",
	OIDCCookie:    "ourtaiko_oidc",
	DefaultReturn: "/upload",
	Categories: []categoryEntry{
		{1, Category{ID: "game", Title: "Game", Genre: "GAME"}},
		{2, Category{ID: "virtual-singer", Title: "Virtual Singer", Genre: "VOCALOID"}},
		{4, Category{ID: "pop", Title: "Pop", Genre: "J-POP"}},
		{8, Category{ID: "classic", Title: "Classic", Genre: "CLASSICAL"}},
		{16, Category{ID: "variety", Title: "Variety", Genre: "VARIETY"}},
		{32, Category{ID: "anime", Title: "Anime", Genre: "ANIME"}},
		// Fanmade's own (0027_fanmade_categories). The game draws a genre it does not know in
		// its default frame.
		{64, Category{ID: "touhou", Title: "Touhou Project", Genre: "TOUHOU"}},
		{128, Category{ID: "gekichumai", Title: "GEKICHUMAI", Genre: "GEKICHUMAI"}},
		{256, Category{ID: "bemani", Title: "BEMANI", Genre: "BEMANI"}},
		{512, Category{ID: "project-sekai", Title: "Project Sekai", Genre: "PROJECT SEKAI"}},
		{1024, Category{ID: "other-rhythm-games", Title: "Other Rhythm Games", Genre: "RHYTHM GAME"}},
		{2048, Category{ID: "bms", Title: "BMS", Genre: "BMS"}},
		{4096, Category{ID: "album", Title: "Album", Genre: "ALBUM"}},
		{8192, Category{ID: "internet", Title: "Internet", Genre: "INTERNET"}},
	},
	// Players suggest categories for its charts, which its managers decide (category_suggestions.go).
	CategorySuggestions: true,
	CategoryIDsSorted:   true,
	PlaylistCategories:  map[string]string{"liked": "pop", "popular": "game", "acclaimed": "classic", "daily": "variety"},
	// Deep blue.
	GenreFrame: 10,
}

// Official is the source of the official game's songs, which its managers keep.
var Official = &Source{
	Official:      true,
	Folder:        "database/",
	SessionCookie: "ourtaiko_db_session",
	OIDCCookie:    "ourtaiko_db_oidc",
	DefaultReturn: "/",
	// In the order of the bits.
	Categories: []categoryEntry{
		{0x01, Category{ID: "namco", Title: "Namco", Genre: "ナムコオリジナル"}},
		{0x02, Category{ID: "pop", Title: "Pop", Genre: "J-POP"}},
		{0x04, Category{ID: "game", Title: "Game", Genre: "ゲームミュージック"}},
		{0x08, Category{ID: "anime", Title: "Anime", Genre: "アニメ"}},
		{0x10, Category{ID: "vocaloid", Title: "Vocaloid", Genre: "VOCALOID"}},
		{0x20, Category{ID: "variety", Title: "Variety", Genre: "バラエティー"}},
		{0x40, Category{ID: "kids", Title: "Kids", Genre: "どうよう"}},
		{0x80, Category{ID: "classics", Title: "Classics", Genre: "クラシック"}},
	},
	PlaylistCategories: map[string]string{"liked": "pop", "popular": "game", "acclaimed": "classics", "daily": "variety"},
	// Orange.
	GenreFrame: 4,
}
