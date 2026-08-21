import type { PointerEventHandler } from 'react'
import type { Article } from '../types/article'
import { Icon } from './Icon'

interface ArticleCardProps {
  article: Article
  tagLabels: string[]
  dragX: number
  dragging: boolean
  isFavorite: boolean
  onDelete: () => void
  onOpen: () => void
  onToggleFavorite: () => void
  onPointerDown: PointerEventHandler<HTMLElement>
  onPointerCancel: PointerEventHandler<HTMLElement>
  onPointerMove: PointerEventHandler<HTMLElement>
  onPointerUp: PointerEventHandler<HTMLElement>
}

export function ArticleCard({
  article,
  tagLabels,
  dragX,
  dragging,
  isFavorite,
  onDelete,
  onOpen,
  onToggleFavorite,
  onPointerDown,
  onPointerCancel,
  onPointerMove,
  onPointerUp,
}: ArticleCardProps) {
  const rotation = Math.max(-8, Math.min(8, dragX / 32))
  const saveOpacity = Math.min(1, Math.max(0, dragX / 90))
  const skipOpacity = Math.min(1, Math.max(0, -dragX / 90))

  return (
    <article
      aria-label={article.title}
      className={`article-card ${dragging ? 'is-dragging' : ''}`}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerCancel}
      style={{ transform: `translate3d(${dragX}px, 0, 0) rotate(${rotation}deg)` }}
    >
      <div className="swipe-indicator swipe-indicator--skip" style={{ opacity: skipOpacity }}>
        SKIP
      </div>
      <div className="swipe-indicator swipe-indicator--save" style={{ opacity: saveOpacity }}>
        SAVE
      </div>
      <div className={`article-visual article-visual--${article.visualTheme}`}>
        <button
          aria-label="削除記事一覧へ移動"
          className="card-icon-button card-icon-button--delete"
          onClick={onDelete}
          onPointerDown={(event) => event.stopPropagation()}
          type="button"
        >
          <Icon name="trash" size={24} />
        </button>
        <button
          aria-label={isFavorite ? 'お気に入りから外す' : 'お気に入りに追加'}
          aria-pressed={isFavorite}
          className={`card-icon-button card-icon-button--favorite ${isFavorite ? 'is-active' : ''}`}
          onClick={onToggleFavorite}
          onPointerDown={(event) => event.stopPropagation()}
          type="button"
        >
          <Icon name="star" size={24} />
        </button>
        <div className="visual-grid" />
        <div className="visual-orbit visual-orbit--one" />
        <div className="visual-orbit visual-orbit--two" />
        <span className="visual-index">{article.id.slice(0, 2).toUpperCase()}</span>
        <span className="visual-label">{article.visualLabel}</span>
      </div>

      <div className="article-content">
        <div className="article-meta">
          <span className="source-mark">{article.sourceInitials}</span>
          <span className="source-name">{article.source}</span>
          <span className="meta-dot" />
          <span>{article.publishedAt}</span>
        </div>

        <div className="article-tags">
          {(tagLabels.length ? tagLabels : ['タグなし']).map((tag) => <span key={tag}>{tag}</span>)}
        </div>
        <h2>{article.title}</h2>
        <div className="article-excerpt">
          <p className="article-summary">{article.summary}</p>
          {article.body.map((paragraph) => <p key={paragraph}>{paragraph}</p>)}
        </div>

        <div className="article-footer">
          <span>{article.readTime} min read</span>
          <button
            className="read-link"
            onClick={onOpen}
            onPointerDown={(event) => event.stopPropagation()}
            type="button"
          >
            読む
            <Icon name="chevron-right" size={18} />
          </button>
        </div>
      </div>
    </article>
  )
}
