package httpapi

// Source is what a song source has of its own, in code that otherwise serves every source
// alike.
type Source struct {
	// Official is whether this is the official source: only its managers add songs, a song may
	// be registered before its files, and its songs carry the official game's identifiers.
	Official bool
	// SessionCookie names the cookie of a website session when the site gives no name of its
	// own. OIDCCookie ties a sign-in at the account center to the browser that began it.
	SessionCookie, OIDCCookie string
	// DefaultReturn is where signing in returns to when it was asked for nowhere, or nowhere
	// allowed.
	DefaultReturn string
	// PageSize is how many charts a page of the chart list has.
	PageSize int
	// Categories is the catalog, in the order and with the names clients have always been
	// given. Genre is the game's box.def #GENRE.
	Categories []categoryEntry
	// CategoryIDsSorted is whether a chart lists its categories by ID rather than in the
	// catalog's order.
	CategoryIDsSorted bool
	// PlaylistCategories files each system playlist under a category, by their IDs.
	PlaylistCategories map[string]string
	// GenreFrame is the game's bar_genre frame (0-12) for this source's own song-select folder.
	GenreFrame int
}

// categoryEntry is a category and its bit in charts.category_flags.
type categoryEntry struct {
	Flag CategoryFlags
	Category
}

// source is the song source this server serves.
func (s *Server) source() *Source {
	if s.Config.Source != nil {
		return s.Config.Source
	}
	return defaultSource
}
