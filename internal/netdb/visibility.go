package netdb

// What the public may see of a song source. A source's own pages and the totals added up over all
// sources must agree, so the rules are written once. c is a chart, m a comment and d a 段位 course.
const (
	// PublishedChart selects the charts that are on show.
	PublishedChart = `c.status='published'`
	// PublishedCommentSubject keeps comments of hidden songs and deleted courses out of sight.
	PublishedCommentSubject = `((m.chart_id IS NOT NULL AND ` + PublishedChart + `) OR (m.dan_id IS NOT NULL AND d.status='published'))`
	// VisibleComment selects live comments on published songs and courses.
	VisibleComment = `m.deleted_at IS NULL AND m.removed_at IS NULL AND ` + PublishedCommentSubject
)
