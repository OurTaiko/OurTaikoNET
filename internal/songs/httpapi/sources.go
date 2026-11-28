package httpapi

// The song sources. A server serves one of them (Config.Source).

// Fanmade is the source of songs players make and upload themselves.
var Fanmade = &Source{
	Folder:        "fanmade/",
	SessionCookie: "ourtaiko_session",
	OIDCCookie:    "ourtaiko_oidc",
	DefaultReturn: "/upload",
	// Eight, one for each of the official source's (0032_fanmade_eight_categories): rhythm-game
	// stands where that has namco, and game is the games that are not rhythm games. The game
	// draws a category in the frame of its genre, so each has the genre of its counterpart. The
	// bits 64, 128, 256, 512, 2048, 4096 and 8192 were categories once and are never used again;
	// what those said of a song its tags say (tags.go).
	Categories: []categoryEntry{
		{1, Category{ID: "game", Title: "Non-Rhythm Game", Genre: "GAME"}},
		{2, Category{ID: "vocaloid", Title: "Vocaloid", Genre: "VOCALOID"}},
		{4, Category{ID: "pop", Title: "Pop", Genre: "J-POP"}},
		{8, Category{ID: "classics", Title: "Classics", Genre: "CLASSICAL"}},
		{16, Category{ID: "variety", Title: "Variety", Genre: "VARIETY"}},
		{32, Category{ID: "anime", Title: "Anime", Genre: "ANIME"}},
		{1024, Category{ID: "rhythm-game", Title: "Rhythm Game", Genre: "NAMCO"}},
		{16384, Category{ID: "kids", Title: "Kids", Genre: "CHILDREN"}},
	},
	// Players suggest categories for its charts, which its managers decide (category_suggestions.go).
	CategorySuggestions: true,
	CategoryIDsSorted:   true,
	PlaylistCategories:  map[string]string{"liked": "pop", "popular": "game", "acclaimed": "classics", "daily": "variety"},
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
