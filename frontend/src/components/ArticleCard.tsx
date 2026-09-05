import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { PointerEventHandler } from 'react'
import type { Article, Tag } from '../types/article'
import { Icon } from './Icon'

interface ArticleCardProps {
  article: Article
  tags: Tag[]
  tagIds: string[]
  dragX: number
  dragging: boolean
  expanded: boolean
  isFavorite: boolean
  onDelete: () => void
  onExpandedChange: (expanded: boolean) => void
  onVisit: () => void
  onToggleFavorite: () => void
  onToggleTag: (tagId: string) => Promise<void>
  onCreateTag: (name: string) => Promise<boolean>
  onPointerDown: PointerEventHandler<HTMLElement>
  onPointerCancel: PointerEventHandler<HTMLElement>
  onPointerMove: PointerEventHandler<HTMLElement>
  onPointerUp: PointerEventHandler<HTMLElement>
}

export function ArticleCard({
  article,
  tags,
  tagIds,
  dragX,
  dragging,
  expanded,
  isFavorite,
  onDelete,
  onExpandedChange,
  onVisit,
  onToggleFavorite,
  onToggleTag,
  onCreateTag,
  onPointerDown,
  onPointerCancel,
  onPointerMove,
  onPointerUp,
}: ArticleCardProps) {
  const previewRef = useRef<HTMLDivElement | null>(null)
  const [hasOverflow, setHasOverflow] = useState(false)
  const [tagPickerOpen, setTagPickerOpen] = useState(false)
  const [tagSearch, setTagSearch] = useState('')
  const [createTagDialogOpen, setCreateTagDialogOpen] = useState(false)
  const [newTagName, setNewTagName] = useState('')
  const [updatingTagId, setUpdatingTagId] = useState<string | null>(null)
  const [creatingTag, setCreatingTag] = useState(false)
  const rotation = Math.max(-8, Math.min(8, dragX / 32))
  const saveOpacity = Math.min(1, Math.max(0, dragX / 90))
  const skipOpacity = Math.min(1, Math.max(0, -dragX / 90))
  const visibleTags = useMemo(() => {
    const query = tagSearch.trim().toLocaleLowerCase('ja')
    return tags
      .filter((tag) => !query || tag.name.toLocaleLowerCase('ja').includes(query))
      .sort((left, right) => {
        if (left.usageCount !== right.usageCount) return right.usageCount - left.usageCount
        const leftUsedAt = left.lastUsedAt ? Date.parse(left.lastUsedAt) : 0
        const rightUsedAt = right.lastUsedAt ? Date.parse(right.lastUsedAt) : 0
        if (leftUsedAt !== rightUsedAt) return rightUsedAt - leftUsedAt
        return left.name.localeCompare(right.name, 'ja')
      })
  }, [tagSearch, tags])
  const normalizedNewTagName = newTagName.trim().toLocaleLowerCase('ja')
  const tagNameAlreadyExists = Boolean(normalizedNewTagName) && tags.some(
    (tag) => tag.name.trim().toLocaleLowerCase('ja') === normalizedNewTagName,
  )

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

  useEffect(() => {
    if (!tagPickerOpen && !createTagDialogOpen) return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      if (createTagDialogOpen) {
        setCreateTagDialogOpen(false)
      } else {
        setTagPickerOpen(false)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [createTagDialogOpen, tagPickerOpen])

  const toggleTag = async (tagId: string) => {
    if (updatingTagId) return
    setUpdatingTagId(tagId)
    try {
      await onToggleTag(tagId)
    } finally {
      setUpdatingTagId(null)
    }
  }

  const createTag = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = newTagName.trim()
    if (!name || tagNameAlreadyExists || creatingTag) return
    setCreatingTag(true)
    try {
      if (await onCreateTag(name)) {
        setTagSearch('')
        setNewTagName('')
        setCreateTagDialogOpen(false)
      }
    } finally {
      setCreatingTag(false)
    }
  }

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
            title="削除記事一覧へ移動"
            type="button"
          >
            <Icon name="trash" size={24} />
          </button>
          <button
            aria-label={isFavorite ? 'お気に入りから外す' : 'お気に入りに追加'}
            aria-keyshortcuts="F"
            aria-pressed={isFavorite}
            className={`card-icon-button card-icon-button--favorite ${isFavorite ? 'is-active' : ''}`}
            onClick={onToggleFavorite}
            onPointerDown={(event) => event.stopPropagation()}
            title={isFavorite ? 'お気に入りから外す（F）' : 'お気に入りに追加（F）'}
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

        <div className="article-tag-control" onPointerDown={(event) => event.stopPropagation()}>
          <div className="article-tags" aria-label="設定中のタグ">
            {tags.filter((tag) => tagIds.includes(tag.id)).map((tag) => (
              <span className="article-tag-chip" key={tag.id}>
                {tag.name}
                <button
                  aria-label={`${tag.name}を削除`}
                  disabled={Boolean(updatingTagId)}
                  onClick={() => void toggleTag(tag.id)}
                  title={`${tag.name}を削除`}
                  type="button"
                >
                  <Icon name="close" size={11} />
                </button>
              </span>
            ))}
            {!tagIds.length && <em>タグなし</em>}
            <button
              aria-expanded={tagPickerOpen}
              aria-label="タグを追加"
              className="article-tag-picker-toggle"
              onClick={() => {
                setTagSearch('')
                setTagPickerOpen((current) => !current)
              }}
              title="タグを追加"
              type="button"
            >
              <Icon name="plus" size={14} />
            </button>
          </div>
          {tagPickerOpen && (
            <>
              <button aria-label="タグ一覧を閉じる" className="tag-picker-scrim" onClick={() => setTagPickerOpen(false)} type="button" />
              <div className="tag-picker article-tag-picker" role="dialog" aria-label={`${article.source}へ割り当てるタグ`}>
                <div className="tag-picker-heading">
                  <strong>タグを選択</strong>
                  <button aria-label="閉じる" onClick={() => setTagPickerOpen(false)} title="閉じる" type="button"><Icon name="close" size={16} /></button>
                </div>
                <label className="tag-picker-search">
                  <span>タグを検索</span>
                  <input
                    autoFocus
                    autoComplete="off"
                    data-1p-ignore="true"
                    onChange={(event) => setTagSearch(event.target.value)}
                    placeholder="タグ名を入力"
                    type="search"
                    value={tagSearch}
                  />
                </label>
                <button
                  className="tag-picker-create"
                  disabled={creatingTag || Boolean(updatingTagId)}
                  aria-label="新しいタグを作成"
                  onClick={() => {
                    setNewTagName(tagSearch.trim())
                    setCreateTagDialogOpen(true)
                  }}
                  title="新しいタグを作成"
                  type="button"
                >
                  <Icon name="plus" size={16} />
                </button>
                <div className="tag-picker-list">
                  {visibleTags.map((tag) => {
                    const selected = tagIds.includes(tag.id)
                    return (
                      <label
                        className={selected ? 'is-active' : ''}
                        key={tag.id}
                      >
                        <input
                          checked={selected}
                          disabled={Boolean(updatingTagId)}
                          onChange={() => void toggleTag(tag.id)}
                          type="checkbox"
                        />
                        <span aria-hidden="true">{selected ? <Icon name="check" size={11} /> : null}</span>
                        {tag.name}
                      </label>
                    )
                  })}
                  {!tags.length && <p>設定可能なタグがありません.</p>}
                  {Boolean(tags.length && !visibleTags.length) && <p>一致するタグがありません.</p>}
                </div>
              </div>
            </>
          )}
          {createTagDialogOpen && (
            <>
              <button
                aria-label="新しいタグの作成をキャンセル"
                className="tag-create-dialog-backdrop"
                onClick={() => setCreateTagDialogOpen(false)}
                type="button"
              />
              <form
                aria-label="新しいタグを作成"
                aria-modal="true"
                className="tag-create-dialog"
                onSubmit={(event) => void createTag(event)}
                role="dialog"
              >
                <div className="tag-create-dialog-heading">
                  <strong>新しいタグを作成</strong>
                  <button aria-label="閉じる" onClick={() => setCreateTagDialogOpen(false)} title="閉じる" type="button"><Icon name="close" size={16} /></button>
                </div>
                <label htmlFor={`new-article-tag-${article.id}`}>タグ名</label>
                <input
                  autoFocus
                  autoComplete="off"
                  data-1p-ignore="true"
                  id={`new-article-tag-${article.id}`}
                  onChange={(event) => setNewTagName(event.target.value)}
                  placeholder="タグ名を入力"
                  type="text"
                  value={newTagName}
                />
                {tagNameAlreadyExists && <p>同じ名前のタグが既にあります.</p>}
                <div className="tag-create-dialog-actions">
                  <button aria-label="キャンセル" disabled={creatingTag} onClick={() => setCreateTagDialogOpen(false)} title="キャンセル" type="button"><Icon name="close" size={16} /></button>
                  <button aria-label={creatingTag ? '作成中' : '作成'} disabled={!newTagName.trim() || tagNameAlreadyExists || creatingTag} title={creatingTag ? '作成中' : '作成'} type="submit">
                    <Icon name="plus" size={16} />
                  </button>
                </div>
              </form>
            </>
          )}
        </div>
        <h2>{articleTitle}</h2>
        <div className={`article-excerpt article-excerpt--preview${hasOverflow ? ' is-overflowing' : ''}`} ref={previewRef}>
          {articleText}
        </div>

        {hasOverflow && (
          <button
            aria-label="記事の続きを読む"
            aria-expanded={expanded}
            className="read-more-button"
            onClick={() => onExpandedChange(true)}
            onPointerDown={(event) => event.stopPropagation()}
            title="続きを読む"
            type="button"
          >
            <Icon name="expand" size={18} />
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
            <button aria-label="閉じる" autoFocus onClick={() => onExpandedChange(false)} title="閉じる" type="button"><Icon name="close" size={17} /></button>
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
