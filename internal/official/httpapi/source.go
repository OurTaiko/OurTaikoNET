package httpapi

// What is this source's own in code it otherwise shares with the fanmade source.
const (
	// The cookie of a website session when the site gives no name of its own.
	defaultSessionCookie = "ourtaiko_db_session"
	// The cookie that ties a sign-in at the account center to the browser that began it.
	oidcCookie = "ourtaiko_db_oidc"
	// Where signing in returns to when it was asked for nowhere, or nowhere allowed.
	defaultReturn = "/"
	// How many charts a page of the chart list has.
	pageSize = 24
	// Whether this is the official source. A chart's JSON has the fields of its source.
	officialSource = true
	// A song's ID in the official source, as chartSelect reads it.
	officialIDColumn = `c.official_id`
)

// The categories of this source. Catalog order and names are what clients have always been
// given. Genre is the game's box.def #GENRE.
const (
	CategoryNamco    CategoryFlags = 0x01
	CategoryPop      CategoryFlags = 0x02
	CategoryGame     CategoryFlags = 0x04
	CategoryAnime    CategoryFlags = 0x08
	CategoryVocaloid CategoryFlags = 0x10
	CategoryVariety  CategoryFlags = 0x20
	CategoryKids     CategoryFlags = 0x40
	CategoryClassics CategoryFlags = 0x80
)

var categoryCatalog = [...]struct {
	Flag CategoryFlags
	Category
}{
	{CategoryNamco, Category{ID: "namco", Title: "Namco", Genre: "ナムコオリジナル"}},
	{CategoryPop, Category{ID: "pop", Title: "Pop", Genre: "J-POP"}},
	{CategoryGame, Category{ID: "game", Title: "Game", Genre: "ゲームミュージック"}},
	{CategoryAnime, Category{ID: "anime", Title: "Anime", Genre: "アニメ"}},
	{CategoryVocaloid, Category{ID: "vocaloid", Title: "Vocaloid", Genre: "VOCALOID"}},
	{CategoryVariety, Category{ID: "variety", Title: "Variety", Genre: "バラエティー"}},
	{CategoryKids, Category{ID: "kids", Title: "Kids", Genre: "どうよう"}},
	{CategoryClassics, Category{ID: "classics", Title: "Classics", Genre: "クラシック"}},
}

const (
	// Whether a chart lists its categories by ID rather than in the catalog's order.
	categoryIDsSorted = false
	// The game's bar_genre frame (0-12) for this source's own song-select folder: orange.
	catalogGenreFrame = 4
)
