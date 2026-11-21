import ReactMarkdown, { type Components } from 'react-markdown'
import remarkBreaks from 'remark-breaks'
import remarkGfm from 'remark-gfm'
import { cn } from '@sitekit/lib/utils'

// GitHub's additions, and a line end in the text is a line end on the page, as in a comment
// there.
const plugins = [remarkGfm, remarkBreaks]

// Links leave the site in a tab of their own; pictures wait until they are near the view.
const components: Components = {
  a: ({ href, title, children }) => (
    <a href={href} title={title} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
  img: ({ src, title, alt = '' }) => <img src={src} title={title} alt={alt} loading="lazy" />,
}

/**
 * Text written in Markdown (with GitHub's tables, strikethrough, task lists, bare web addresses
 * as links, and line ends kept), shown as the site's own text. HTML in the text is shown as it was
 * written, never run: this is for what staff write, and still never trusts it as markup.
 * Carries a Markdown parser, so pages load it lazily.
 */
export default function Markdown({ text, className }: { text: string; className?: string }) {
  return (
    <div className={cn('markdown', className)}>
      <ReactMarkdown remarkPlugins={plugins} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  )
}
