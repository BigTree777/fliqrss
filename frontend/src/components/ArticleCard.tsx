import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { PointerEventHandler } from 'react'
import type { Article } from '../types/article'
import { Icon } from './Icon'

interface ArticleCardProps {
  article: Article
  tagLabels: string[]
  dragX: number
  dragging: boolean
  expanded: boolean
  isFavorite: boolean
  onDelete: () => void
  onExpandedChange: (expanded: boolean) => void
  onVisit: () => void
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
  expanded,
  isFavorite,
  onDelete,
  onExpandedChange,
  onVisit,
  onToggleFavorite,
  onPointerDown,
  onPointerCancel,
  onPointerMove,
  onPointerUp,
}: ArticleCardProps) {
  const previewRef = useRef<HTMLDivElement | null>(null)
  const [hasOverflow, setHasOverflow] = useState(false)
  const rotation = Math.max(-8, Math.min(8, dragX / 32))
  const saveOpacity = Math.min(1, Math.max(0, dragX / 90))
  const skipOpacity = Math.min(1, Math.max(0, -dragX / 90))

  useLayoutEffect(() => {
    const preview = previewRef.current
    if (!preview) return

    const updateOverflow = () => setHasOverflow(preview.scrollHeight > preview.clientHeight + 1)
    updateOverflow()
    const observer = new ResizeObserver(updateOverflow)
    observer.observe(preview)
    return () => observer.disconnect()
  }, [article, expanded])

  useEffect(() => {
    if (!expanded) return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onExpandedChange(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [expanded, onExpandedChange])

  const articleTitle = article.url ? (
    <a
      href={article.url}
      onClick={onVisit}
      onPointerDown={(event) => event.stopPropagation()}
      rel="noreferrer"
      target="_blank"
    >
      {article.title}
    </a>
  ) : article.title

  const articleText = (
    <>
      <p className="article-summary">{article.summary}</p>
      {article.body.map((paragraph, index) => <p key={`${index}-${paragraph}`}>{paragraph}</p>)}
    </>
  )

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
      {!expanded && <div className="article-content">
        <div className="card-toolbar">
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
        </div>
        <div className="article-meta">
          <span className="source-mark">{article.sourceInitials}</span>
          <span className="source-name">{article.source}</span>
          <span className="meta-dot" />
          <span>{article.publishedAt}</span>
        </div>

        <div className="article-tags">
          {(tagLabels.length ? tagLabels : ['タグなし']).map((tag) => <span key={tag}>{tag}</span>)}
        </div>
        <h2>{articleTitle}</h2>
        <div className={`article-excerpt article-excerpt--preview${hasOverflow ? ' is-overflowing' : ''}`} ref={previewRef}>
          {articleText}
        </div>

        {hasOverflow && (
          <button
            aria-expanded={expanded}
            className="read-more-button"
            onClick={() => onExpandedChange(true)}
            onPointerDown={(event) => event.stopPropagation()}
            type="button"
          >
            続きを読む
          </button>
        )}

        <div className="article-footer">
          <span>{article.readTime} min read</span>
        </div>
      </div>}

      {expanded && (
        <section
          aria-label={`${article.title}の全文`}
          aria-modal="true"
          className="article-detail"
          onPointerDown={(event) => event.stopPropagation()}
          role="dialog"
        >
          <div className="article-detail-heading">
            <span>{article.source}</span>
            <button autoFocus onClick={() => onExpandedChange(false)} type="button">閉じる</button>
          </div>
          <div className="article-detail-scroll">
            <h2>{articleTitle}</h2>
            <div className="article-excerpt">{articleText}</div>
          </div>
        </section>
      )}
    </article>
  )
}
