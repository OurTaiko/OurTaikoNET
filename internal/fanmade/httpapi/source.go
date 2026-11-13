package httpapi

// What is this source's own in code it otherwise shares with the official source.
const (
	// The cookie of a website session when the site gives no name of its own.
	defaultSessionCookie = "ourtaiko_session"
	// The cookie that ties a sign-in at the account center to the browser that began it.
	oidcCookie = "ourtaiko_oidc"
	// Where signing in returns to when it was asked for nowhere, or nowhere allowed.
	defaultReturn = "/upload"
	// How many charts a page of the chart list has.
	pageSize = 12
	// Whether this is the official source. A chart's JSON has the fields of its source.
	officialSource = false
	// A song's ID in the official source, as chartSelect reads it: this source has none.
	officialIDColumn = `NULL::text`
)

// The categories of this source. Catalog order and names are what clients have always been
// given. Genre is the game's box.def #GENRE.
const (
	CategoryGame          CategoryFlags = 1
	CategoryVirtualSinger CategoryFlags = 2
	CategoryPop           CategoryFlags = 4
	CategoryClassics      CategoryFlags = 8
	CategoryVariety       CategoryFlags = 16
	CategoryAnime         CategoryFlags = 32
)

var categoryCatalog = [...]struct {
	Flag CategoryFlags
	Category
}{
	{CategoryGame, Category{ID: "game", Title: "Game", Genre: "GAME"}},
	{CategoryVirtualSinger, Category{ID: "virtual-singer", Title: "Virtual Singer", Genre: "VOCALOID"}},
	{CategoryPop, Category{ID: "pop", Title: "Pop", Genre: "J-POP"}},
	{CategoryClassics, Category{ID: "classic", Title: "Classic", Genre: "CLASSICAL"}},
	{CategoryVariety, Category{ID: "variety", Title: "Variety", Genre: "VARIETY"}},
	{CategoryAnime, Category{ID: "anime", Title: "Anime", Genre: "ANIME"}},
}

const (
	// Whether a chart lists its categories by ID rather than in the catalog's order.
	categoryIDsSorted = true
	// The game's bar_genre frame (0-12) for this source's own song-select folder: deep blue.
	catalogGenreFrame = 10
)
