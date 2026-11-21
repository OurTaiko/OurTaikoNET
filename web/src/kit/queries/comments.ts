// The comments under a chart, a 段位 course or an announcement, page after page, or the one
// thread a permalink shows. The subject's addresses are taken when it is made, on a page of its
// host: by the time a later page is asked for the visitor may be in another area.
import { infiniteQueryOptions, type InfiniteData } from '@tanstack/react-query'
import { api } from '../api/client'
import type { CommentPage, CommentSort, CommentThread } from '../api/types'
import { queryClient } from '../query/client'

export type CommentsRead = Pick<CommentPage, 'items' | 'total'>
export type CommentPages = InfiniteData<CommentsRead, number>

/** What of a subject says where its comments are kept and read. */
type Read = {
  host: string
  path: string
  comments: (query: { sort: string; page: number }) => string
  commentThread: (commentId: string, sort: string) => string
}

export const commentKeys = {
  // The subject's identity is its host and its path; its callbacks are rebuilt each render.
  subject: ({ host, path }: Pick<Read, 'host' | 'path'>) => [host, 'comments', path] as const,
  section: (subject: Pick<Read, 'host' | 'path'>, focusId: string, sort: CommentSort) =>
    [...commentKeys.subject(subject), focusId, sort] as const,
}

export const commentQueries = {
  /** Another order keeps the comments on show until it comes; another subject or thread does not. */
  section: (subject: Read, focusId: string | undefined, sort: CommentSort) => {
    const { path } = subject
    const focus = focusId ?? ''
    const thread = focusId ? subject.commentThread(focusId, sort) : ''
    return infiniteQueryOptions({
      queryKey: commentKeys.section(subject, focus, sort),
      queryFn: ({ pageParam, signal }): Promise<CommentsRead> =>
        thread
          ? api<CommentThread>(thread, { signal }).then((r) => ({ items: [r.item], total: 1 }))
          : api<CommentPage>(subject.comments({ sort, page: pageParam }), { signal }),
      initialPageParam: 1,
      // The page asks for more only while fewer comments are shown than the total says.
      getNextPageParam: (_last, _pages, page) => page + 1,
      placeholderData: (previous, previousQuery) =>
        previousQuery?.queryKey[2] === path && previousQuery.queryKey[3] === focus
          ? previous
          : undefined,
    })
  },
}

/** After a comment changed: the subject's other orders are out of date, and asked again when shown. */
export const invalidateSubjectComments = (subject: Pick<Read, 'host' | 'path'>) =>
  queryClient.invalidateQueries({ queryKey: commentKeys.subject(subject), refetchType: 'none' })
