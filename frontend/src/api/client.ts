import type { Article, ArticleAction, Source, Tag } from '../types/article'

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
  if (init?.body) headers.set('Content-Type', 'application/json')

  const response = await fetch(`${API_BASE_URL}${path}`, { ...init, headers })
  if (!response.ok) {
    let body: ErrorEnvelope = {}
    try {
      body = await response.json() as ErrorEnvelope
    } catch {
      // The fallback below is used when the server did not return JSON.
    }
    throw new ApiError(
      response.status,
      body.error?.code ?? 'request_failed',
      body.error?.message ?? `API request failed with status ${response.status}`,
    )
  }
  if (response.status === 204) return undefined as T
  const envelope = await response.json() as DataEnvelope<T>
  return envelope.data
}

export const api = {
  listArticles: () => request<Article[]>('/articles?state=all'),
  updateArticleState: (id: string, action: ArticleAction) => request<Article>(`/articles/${encodeURIComponent(id)}/state`, {
    method: 'PATCH',
    body: JSON.stringify({ action }),
  }),
  resetSkipped: () => request<{ restored: number }>('/articles/reset-skipped', { method: 'POST' }),

  listSources: () => request<Source[]>('/sources'),
  createSource: (name: string, url: string) => request<Source>('/sources', {
    method: 'POST',
    body: JSON.stringify({ name, url }),
  }),
  updateSource: (id: string, values: { name?: string; enabled?: boolean }) => request<Source>(`/sources/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(values),
  }),
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
