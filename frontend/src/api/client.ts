import type { Article, ArticleAction, ArticlePage, ArticleStats, Source, Tag } from '../types/article'

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? '/api/v1').replace(/\/$/, '')

interface DataEnvelope<T> {
  data: T
}

interface ErrorEnvelope {
  error?: {
    code?: string
    message?: string
  }
}

async function apiErrorFromResponse(response: Response): Promise<ApiError> {
  let body: ErrorEnvelope = {}
  try {
    body = await response.json() as ErrorEnvelope
  } catch {
    // The fallback below is used when the server did not return JSON.
  }
  return new ApiError(
    response.status,
    body.error?.code ?? 'request_failed',
    body.error?.message ?? `API request failed with status ${response.status}`,
  )
}

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')

  const response = await fetch(`${API_BASE_URL}${path}`, { ...init, headers })
  if (!response.ok) {
    throw await apiErrorFromResponse(response)
  }
  if (response.status === 204) return undefined as T
  const envelope = await response.json() as DataEnvelope<T>
  return envelope.data
}

export interface OPMLImportResult {
  total: number
  added: number
  duplicates: number
  failed: number
  tagsCreated: number
  failures: SourceFailure[]
}

export interface SourceFailure {
  sourceId?: string
  name: string
  url: string
  stage: 'validation' | 'fetch' | 'save'
  reason: string
}

export interface SourceRefreshResult {
  sources: number
  refreshed: number
  added: number
  failed: number
  initialRefreshed: number
  initialFailed: number
  retried: number
  recovered: number
  failures: SourceFailure[]
}

export interface ArticlePageQuery {
  state?: 'feed' | 'favorite' | 'saved' | 'deleted' | 'read' | 'all'
  sourceId?: string
  tagId?: string
  untagged?: boolean
  maxAgeDays?: number
  cursor?: string
  limit?: number
}

function articlePagePath(query: ArticlePageQuery): string {
  const parameters = new URLSearchParams()
  parameters.set('state', query.state ?? 'feed')
  parameters.set('limit', String(query.limit ?? 20))
  if (query.sourceId) parameters.set('sourceId', query.sourceId)
  if (query.tagId) parameters.set('tagId', query.tagId)
  if (query.untagged) parameters.set('untagged', 'true')
  if (query.maxAgeDays) parameters.set('maxAgeDays', String(query.maxAgeDays))
  if (query.cursor) parameters.set('cursor', query.cursor)
  return `/articles/page?${parameters.toString()}`
}

export const api = {
  listArticlePage: (query: ArticlePageQuery) => request<ArticlePage>(articlePagePath(query)),
  articleStats: (maxAgeDays?: number) => request<ArticleStats>(maxAgeDays ? `/articles/stats?maxAgeDays=${maxAgeDays}` : '/articles/stats'),
  updateArticleState: (id: string, action: ArticleAction) => request<Article>(`/articles/${encodeURIComponent(id)}/state`, {
    method: 'PATCH',
    body: JSON.stringify({ action }),
  }),
  markAllRead: (sourceId: string, maxAgeDays?: number) => {
    const parameters = new URLSearchParams({ sourceId })
    if (maxAgeDays) parameters.set('maxAgeDays', String(maxAgeDays))
    return request<{ markedRead: number }>(`/articles/mark-all-read?${parameters.toString()}`, { method: 'POST' })
  },
  resetSkipped: (sourceId: string) => request<{ restored: number }>(`/articles/reset-skipped?sourceId=${encodeURIComponent(sourceId)}`, { method: 'POST' }),

  listSources: () => request<Source[]>('/sources'),
  createSource: (name: string, url: string) => request<Source>('/sources', {
    method: 'POST',
    body: JSON.stringify({ name, url }),
  }),
  importOPML: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return request<OPMLImportResult>('/sources/import-opml', { method: 'POST', body: form })
  },
  exportOPML: async () => {
    const response = await fetch(`${API_BASE_URL}/sources/export-opml`)
    if (!response.ok) throw await apiErrorFromResponse(response)
    return response.blob()
  },
  updateSource: (id: string, values: { name?: string; enabled?: boolean }) => request<Source>(`/sources/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(values),
  }),
  reorderSources: (sourceIds: string[]) => request<Source[]>('/sources/order', {
    method: 'PUT',
    body: JSON.stringify({ sourceIds }),
  }),
  refreshAllSources: () => request<SourceRefreshResult>('/sources/refresh', { method: 'POST' }),
  deleteSource: (id: string) => request<void>(`/sources/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  setSourceTags: (id: string, tagIds: string[]) => request<Source>(`/sources/${encodeURIComponent(id)}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tagIds }),
  }),

  listTags: () => request<Tag[]>('/tags'),
  createTag: (name: string) => request<Tag>('/tags', {
    method: 'POST',
    body: JSON.stringify({ name }),
  }),
  updateTag: (id: string, name: string) => request<Tag>(`/tags/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify({ name }),
  }),
  deleteTag: (id: string) => request<void>(`/tags/${encodeURIComponent(id)}`, { method: 'DELETE' }),
}
