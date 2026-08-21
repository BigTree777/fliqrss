export type VisualTheme = 'cobalt' | 'coral' | 'forest' | 'violet' | 'amber' | 'aqua'

export interface Article {
  id: string
  sourceId: string
  source: string
  sourceInitials: string
  publishedAt: string
  readTime: number
  title: string
  url?: string
  summary: string
  body: string[]
  visualLabel: string
  visualTheme: VisualTheme
  tagIds: string[]
  state: ArticleState
}

export interface ArticleState {
  read: boolean
  skipped: boolean
  saved: boolean
  favorite: boolean
  deleted: boolean
}

export type ArticleAction =
  | 'read'
  | 'unread'
  | 'skip'
  | 'save'
  | 'unsave'
  | 'favorite'
  | 'unfavorite'
  | 'delete'
  | 'restore'

export interface ArticlePage {
  items: Article[]
  nextCursor?: string
  total: number
}

export interface ArticleStats {
  feed: number
  favorite: number
  saved: number
  deleted: number
  skipped: number
  untaggedFeed: number
  sourceFeedCounts: Record<string, number>
  tagFeedCounts: Record<string, number>
}

export interface Source {
  id: string
  name: string
  url: string
  format: 'rss' | 'atom'
  enabled: boolean
  tagIds: string[]
  articleCount: number
  lastFetchedAt: string | null
  createdAt: string
}

export interface Tag {
  id: string
  name: string
  createdAt: string
}
