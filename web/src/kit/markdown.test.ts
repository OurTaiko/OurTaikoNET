import { createElement } from 'react'
import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import Markdown from './markdown'

const shown = (text: string) => renderToStaticMarkup(createElement(Markdown, { text }))

describe('Markdown', () => {
  it('shows headings, emphasis, lists and tables', () => {
    const html = shown(
      '## Tonight\n\n**Closed** at nine.\n\n- one\n- two\n\n| a | b |\n|---|---|\n| 1 | 2 |',
    )
    expect(html).toContain('<h2>Tonight</h2>')
    expect(html).toContain('<strong>Closed</strong>')
    expect(html).toContain('<li>one</li>')
    expect(html).toContain('<td>1</td>')
  })

  it('keeps the line ends of a paragraph', () => {
    expect(shown('one\ntwo')).toContain('one<br/>')
  })

  it('opens links, written out or bare, in a tab of their own', () => {
    const link = '<a href="https://net.ourtaiko.org/" target="_blank" rel="noopener noreferrer">'
    expect(shown('[the site](https://net.ourtaiko.org/)')).toContain(link)
    expect(shown('See https://net.ourtaiko.org/ now')).toContain(link)
  })

  it('never takes HTML or script addresses from the text', () => {
    const html = shown(
      '<script>alert(1)</script><b onclick="x()">bold</b>\n\n[x](javascript:alert(1))',
    )
    expect(html).not.toContain('<script')
    expect(html).not.toContain('<b')
    expect(html).not.toContain('javascript:')
  })
})

describe('Markdown by a member', () => {
  const byMember = (text: string) =>
    renderToStaticMarkup(createElement(Markdown, { text, by: 'member' }))

  it('marks links as the writer’s', () => {
    expect(byMember('See https://net.ourtaiko.org/ now')).toContain(
      'rel="noopener noreferrer nofollow ugc"',
    )
  })

  it('shows a picture, fetched without saying where it is shown', () => {
    const html = byMember('![a drum](https://example.com/drum.png)')
    expect(html).toContain('<img src="https://example.com/drum.png" alt="a drum"')
    expect(html).toContain('referrerPolicy="no-referrer"')
  })

  it('never takes HTML or script addresses from the text', () => {
    const html = byMember('<img src=x onerror="x()">\n\n[x](javascript:alert(1))')
    expect(html).not.toContain('onerror="')
    expect(html).not.toContain('javascript:')
  })
})
