package httpapi

// defaultSource is this package's own source: the Fanmade one.
var defaultSource = &Source{
	SessionCookie: "ourtaiko_session",
	OIDCCookie:    "ourtaiko_oidc",
	DefaultReturn: "/upload",
	PageSize:      12,
	Categories: []categoryEntry{
		{1, Category{ID: "game", Title: "Game", Genre: "GAME"}},
		{2, Category{ID: "virtual-singer", Title: "Virtual Singer", Genre: "VOCALOID"}},
		{4, Category{ID: "pop", Title: "Pop", Genre: "J-POP"}},
		{8, Category{ID: "classic", Title: "Classic", Genre: "CLASSICAL"}},
		{16, Category{ID: "variety", Title: "Variety", Genre: "VARIETY"}},
		{32, Category{ID: "anime", Title: "Anime", Genre: "ANIME"}},
	},
	CategoryIDsSorted:  true,
	PlaylistCategories: map[string]string{"liked": "pop", "popular": "game", "acclaimed": "classic", "daily": "variety"},
	// Deep blue.
	GenreFrame: 10,
}
