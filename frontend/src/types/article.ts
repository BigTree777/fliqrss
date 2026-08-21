export type VisualTheme = 'cobalt' | 'coral' | 'forest' | 'violet' | 'amber' | 'aqua'

export interface Article {
  id: string
  source: string
  sourceInitials: string
  publishedAt: string
  readTime: number
  title: string
  summary: string
  body: string[]
  visualLabel: string
  visualTheme: VisualTheme
}
