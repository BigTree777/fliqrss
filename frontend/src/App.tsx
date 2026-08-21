import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, ApiError } from './api/client'
import { ArticleCard } from './components/ArticleCard'
import { Icon } from './components/Icon'
import type { Article, ArticleAction, Source, Tag } from './types/article'

type TagFilter = string
type FilterMode = 'source' | 'tag'
type SwipeAction = 'skip' | 'save'
type LibraryMode = 'favorite' | 'saved' | 'deleted'

const ALL_TAGS = '__all__'
const UNTAGGED = '__untagged__'
const ALL_SOURCES = '__all_sources__'
const SWIPE_THRESHOLD = 92

function libraryModeFromHash(): LibraryMode | null {
  if (window.location.hash === '#/favorites') return 'favorite'
  if (window.location.hash === '#/saved') return 'saved'
  if (window.location.hash === '#/deleted') return 'deleted'
  return null
}

function App() {
  const [filterMode, setFilterMode] = useState<FilterMode>('source')
  const [source, setSource] = useState(ALL_SOURCES)
  const [articles, setArticles] = useState<Article[]>([])
  const [managedSources, setManagedSources] = useState<Source[]>([])
  const [sourceManagerOpen, setSourceManagerOpen] = useState(() => window.location.hash === '#/sources')
  const [newSourceName, setNewSourceName] = useState('')
  const [newSourceUrl, setNewSourceUrl] = useState('')
  const [openTagPickerSourceId, setOpenTagPickerSourceId] = useState<string | null>(null)
  const [tag, setTag] = useState<TagFilter>(ALL_TAGS)
  const [managedTags, setManagedTags] = useState<Tag[]>([])
  const [tagManagerOpen, setTagManagerOpen] = useState(() => window.location.hash === '#/tags')
  const [menuOpen, setMenuOpen] = useState(false)
  const [newTagName, setNewTagName] = useState('')
  const [editingTagId, setEditingTagId] = useState<string | null>(null)
  const [editingTagName, setEditingTagName] = useState('')
  const [dragX, setDragX] = useState(0)
  const [dragging, setDragging] = useState(false)
  const [animating, setAnimating] = useState(false)
  const [openArticle, setOpenArticle] = useState<Article | null>(null)
  const [libraryMode, setLibraryMode] = useState<LibraryMode | null>(() => libraryModeFromHash())
  const [notice, setNotice] = useState('')
  const [apiError, setApiError] = useState('')
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState(false)
  const pointerStart = useRef(0)
  const activePointer = useRef<number | null>(null)
  const animationTimer = useRef<number | null>(null)
  const noticeTimer = useRef<number | null>(null)

  const deletedIds = useMemo(() => new Set(articles.filter((item) => item.state.deleted).map((item) => item.id)), [articles])
  const favoriteIds = useMemo(() => new Set(articles.filter((item) => item.state.favorite).map((item) => item.id)), [articles])
  const savedIds = useMemo(() => new Set(articles.filter((item) => item.state.saved).map((item) => item.id)), [articles])
  const readIds = useMemo(() => new Set(articles.filter((item) => item.state.read).map((item) => item.id)), [articles])
  const skippedIds = useMemo(() => new Set(articles.filter((item) => item.state.skipped).map((item) => item.id)), [articles])
  const tagIdsForSource = useCallback((sourceId: string) => (
    managedSources.find((item) => item.id === sourceId)?.tagIds ?? []
  ), [managedSources])
  const selectionArticles = useMemo(() => {
    if (filterMode === 'source') {
      return source === ALL_SOURCES ? articles : articles.filter((article) => article.sourceId === source)
    }
    if (tag === ALL_TAGS) return articles
    if (tag === UNTAGGED) return articles.filter((article) => article.tagIds.length === 0)
    return articles.filter((article) => article.tagIds.includes(tag))
  }, [filterMode, source, tag, tagIdsForSource])

  const untaggedCount = useMemo(
    () => articles.filter((article) => article.tagIds.length === 0).length,
    [articles],
  )

  const tagNamesForSource = useCallback((sourceId: string) => tagIdsForSource(sourceId).map(
    (tagId) => managedTags.find((item) => item.id === tagId)?.name,
  ).filter((name): name is string => Boolean(name)), [managedTags, tagIdsForSource])

  const tagOptions = useMemo(() => [
    { id: ALL_TAGS, name: 'すべて' },
    ...managedTags,
    ...(untaggedCount ? [{ id: UNTAGGED, name: 'タグなし' }] : []),
  ], [managedTags, untaggedCount])

  const sourceOptions = useMemo(() => [
    { id: ALL_SOURCES, name: 'すべてのソース' },
    ...managedSources.map((item) => ({ id: item.id, name: item.name })),
  ], [managedSources])

  const filteredArticles = useMemo(
    () => selectionArticles.filter(
      (article) => !readIds.has(article.id) && !savedIds.has(article.id) && !deletedIds.has(article.id),
    ),
    [deletedIds, readIds, savedIds, selectionArticles],
  )

  const currentArticle = filteredArticles[0]

  const showNotice = useCallback((message: string) => {
    setNotice(message)
    if (noticeTimer.current) window.clearTimeout(noticeTimer.current)
    noticeTimer.current = window.setTimeout(() => setNotice(''), 1800)
  }, [])

  const showApiError = useCallback((error: unknown) => {
    const message = error instanceof ApiError
      ? error.message
      : error instanceof Error
        ? error.message
        : 'バックエンドと通信できません'
    setApiError(message)
    showNotice('操作に失敗しました')
  }, [showNotice])

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [nextArticles, nextSources, nextTags] = await Promise.all([
        api.listArticles(),
        api.listSources(),
        api.listTags(),
      ])
      setArticles(nextArticles)
      setManagedSources(nextSources)
      setManagedTags(nextTags)
      setApiError('')
    } catch (error) {
      showApiError(error)
    } finally {
      setLoading(false)
    }
  }, [showApiError])

  const replaceArticle = useCallback((nextArticle: Article) => {
    setArticles((current) => current.map((item) => item.id === nextArticle.id ? nextArticle : item))
    setOpenArticle((current) => current?.id === nextArticle.id ? nextArticle : current)
  }, [])

  const updateArticleState = useCallback(async (articleId: string, action: ArticleAction) => {
    const nextArticle = await api.updateArticleState(articleId, action)
    replaceArticle(nextArticle)
    setApiError('')
    return nextArticle
  }, [replaceArticle])

  const navigateToLibrary = (mode: LibraryMode) => {
    const paths: Record<LibraryMode, string> = {
      favorite: '#/favorites',
      saved: '#/saved',
      deleted: '#/deleted',
    }
    setMenuOpen(false)
    setOpenTagPickerSourceId(null)
    window.location.hash = paths[mode]
  }

  const navigateToReader = () => {
    setMenuOpen(false)
    setOpenTagPickerSourceId(null)
    window.location.hash = '#/'
  }

  const navigateToTags = () => {
    setMenuOpen(false)
    setOpenTagPickerSourceId(null)
    window.location.hash = '#/tags'
  }

  const navigateToSources = () => {
    setMenuOpen(false)
    setOpenTagPickerSourceId(null)
    window.location.hash = '#/sources'
  }

  const addSource = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = newSourceName.trim()
    const url = newSourceUrl.trim()
    if (!name || !url) return
    setPending(true)
    try {
      const created = await api.createSource(name, url)
      const nextArticles = await api.listArticles()
      setManagedSources((current) => [...current, created])
      setArticles(nextArticles)
      setNewSourceName('')
      setNewSourceUrl('')
      setApiError('')
      showNotice('ニュースソースを追加しました')
    } catch (error) {
      showApiError(error)
    } finally {
      setPending(false)
    }
  }

  const toggleSourceEnabled = async (targetSource: Source) => {
    try {
      const updated = await api.updateSource(targetSource.id, { enabled: !targetSource.enabled })
      setManagedSources((current) => current.map((item) => item.id === updated.id ? updated : item))
      setApiError('')
    } catch (error) {
      showApiError(error)
    }
  }

  const deleteSource = async (targetSource: Source) => {
    try {
      await api.deleteSource(targetSource.id)
      setManagedSources((current) => current.filter((item) => item.id !== targetSource.id))
      setArticles((current) => current.filter((item) => item.sourceId !== targetSource.id))
      if (source === targetSource.id) setSource(ALL_SOURCES)
      if (openTagPickerSourceId === targetSource.id) setOpenTagPickerSourceId(null)
      setApiError('')
      showNotice('ニュースソースを削除しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const addTag = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = newTagName.trim()
    if (!name) return
    try {
      const created = await api.createTag(name)
      setManagedTags((current) => [...current, created])
      setNewTagName('')
      setApiError('')
      showNotice('タグを追加しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const startEditingTag = (item: Tag) => {
    setEditingTagId(item.id)
    setEditingTagName(item.name)
  }

  const saveTagName = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = editingTagName.trim()
    if (!editingTagId || !name) return
    try {
      const updated = await api.updateTag(editingTagId, name)
      setManagedTags((current) => current.map((item) => item.id === updated.id ? updated : item))
      setEditingTagId(null)
      setApiError('')
      showNotice('タグ名を変更しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const deleteTag = async (tagId: string) => {
    try {
      await api.deleteTag(tagId)
      setManagedTags((current) => current.filter((item) => item.id !== tagId))
      setManagedSources((current) => current.map((item) => ({
        ...item,
        tagIds: item.tagIds.filter((id) => id !== tagId),
      })))
      setArticles((current) => current.map((item) => ({
        ...item,
        tagIds: item.tagIds.filter((id) => id !== tagId),
      })))
      if (tag === tagId) setTag(ALL_TAGS)
      if (editingTagId === tagId) setEditingTagId(null)
      setApiError('')
      showNotice('タグを削除しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const toggleSourceTag = async (targetSource: Source, tagId: string) => {
    const nextTagIds = targetSource.tagIds.includes(tagId)
      ? targetSource.tagIds.filter((id) => id !== tagId)
      : [...targetSource.tagIds, tagId]
    try {
      const updated = await api.setSourceTags(targetSource.id, nextTagIds)
      setManagedSources((current) => current.map((item) => item.id === updated.id ? updated : item))
      setArticles((current) => current.map((item) => (
        item.sourceId === updated.id ? { ...item, tagIds: updated.tagIds } : item
      )))
      setApiError('')
    } catch (error) {
      showApiError(error)
    }
  }

  const completeSwipe = useCallback(
    async (action: SwipeAction) => {
      if (!currentArticle || animating) return

      setAnimating(true)
      setDragging(false)
      try {
        const nextArticle = await api.updateArticleState(currentArticle.id, action)
        const direction = action === 'save' ? 1 : -1
        setDragX(direction * Math.max(window.innerWidth * 0.7, 680))
        showNotice(action === 'save' ? 'あとで読むに保存しました' : '既読にしました')
        animationTimer.current = window.setTimeout(() => {
          replaceArticle(nextArticle)
          setDragX(0)
          setAnimating(false)
        }, 260)
      } catch (error) {
        setDragX(0)
        setAnimating(false)
        showApiError(error)
      }
    },
    [animating, currentArticle, replaceArticle, showApiError, showNotice],
  )

  const selectTag = (nextTag: TagFilter) => {
    if (animating) return
    setTag(nextTag)
    setDragX(0)
  }

  const selectSource = (nextSource: string) => {
    if (animating) return
    setSource(nextSource)
    setDragX(0)
  }

  const selectFilterMode = (nextMode: FilterMode) => {
    if (animating) return
    setFilterMode(nextMode)
    setDragX(0)
  }

  const openDetail = async (article: Article) => {
    setOpenArticle(article)
    if (article.state.read) return
    try {
      await updateArticleState(article.id, 'read')
    } catch (error) {
      showApiError(error)
    }
  }

  const toggleFavorite = async (articleId: string) => {
    const article = articles.find((item) => item.id === articleId)
    if (!article) return
    try {
      await updateArticleState(articleId, article.state.favorite ? 'unfavorite' : 'favorite')
      showNotice(article.state.favorite ? 'お気に入りから外しました' : 'お気に入りに追加しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const deleteArticle = async (articleId: string) => {
    try {
      await updateArticleState(articleId, 'delete')
      showNotice('削除記事一覧へ移動しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const removeFromLibrary = async (articleId: string) => {
    const action = libraryMode === 'favorite'
      ? 'unfavorite'
      : libraryMode === 'saved'
        ? 'unsave'
        : 'restore'
    try {
      await updateArticleState(articleId, action)
      showNotice(libraryMode === 'favorite'
        ? 'お気に入りから外しました'
        : libraryMode === 'saved'
          ? '保存を解除しました'
          : '削除記事一覧から戻しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const restoreSkippedArticles = async () => {
    try {
      const result = await api.resetSkipped()
      setArticles(await api.listArticles())
      setDragX(0)
      setApiError('')
      showNotice(`${result.restored}件のスキップを解除しました`)
    } catch (error) {
      showApiError(error)
    }
  }

  useEffect(() => {
    void loadData()
  }, [loadData])

  useEffect(() => {
    const handleHashChange = () => {
      setLibraryMode(libraryModeFromHash())
      setTagManagerOpen(window.location.hash === '#/tags')
      setSourceManagerOpen(window.location.hash === '#/sources')
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (openTagPickerSourceId) {
        if (event.key === 'Escape') setOpenTagPickerSourceId(null)
        return
      }

      if (menuOpen) {
        if (event.key === 'Escape') setMenuOpen(false)
        return
      }

      if (openArticle) {
        if (event.key === 'Escape') setOpenArticle(null)
        return
      }

      if (libraryMode || sourceManagerOpen || tagManagerOpen) return

      if (event.key === 'ArrowLeft') completeSwipe('skip')
      if (event.key === 'ArrowRight') completeSwipe('save')
      if (event.key === 'Enter' && currentArticle) openDetail(currentArticle)
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [completeSwipe, currentArticle, libraryMode, menuOpen, openArticle, openTagPickerSourceId, sourceManagerOpen, tagManagerOpen])

  useEffect(
    () => () => {
      if (animationTimer.current) window.clearTimeout(animationTimer.current)
      if (noticeTimer.current) window.clearTimeout(noticeTimer.current)
    },
    [],
  )

  const handlePointerDown: React.PointerEventHandler<HTMLElement> = (event) => {
    if (animating) return
    activePointer.current = event.pointerId
    pointerStart.current = event.clientX
    setDragging(true)
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  const handlePointerMove: React.PointerEventHandler<HTMLElement> = (event) => {
    if (!dragging || activePointer.current !== event.pointerId) return
    setDragX(event.clientX - pointerStart.current)
  }

  const handlePointerCancel: React.PointerEventHandler<HTMLElement> = (event) => {
    if (activePointer.current !== event.pointerId) return
    activePointer.current = null
    setDragging(false)
    setDragX(0)
  }

  const handlePointerUp: React.PointerEventHandler<HTMLElement> = (event) => {
    if (activePointer.current !== event.pointerId) return
    const finalDragX = event.clientX - pointerStart.current
    activePointer.current = null
    setDragging(false)

    if (finalDragX > SWIPE_THRESHOLD) {
      completeSwipe('save')
    } else if (finalDragX < -SWIPE_THRESHOLD) {
      completeSwipe('skip')
    } else {
      setDragX(0)
    }
  }

  const processedCount = selectionArticles.length - filteredArticles.length
  const progress = selectionArticles.length
    ? Math.min(100, (processedCount / selectionArticles.length) * 100)
    : 0
  const libraryIds = libraryMode === 'favorite'
    ? favoriteIds
    : libraryMode === 'saved'
      ? savedIds
      : deletedIds
  const libraryArticles = articles.filter((article) => libraryIds.has(article.id))
  const remainingTagCount = (targetTag: TagFilter) => articles.filter((article) => {
    const articleTagIds = article.tagIds
    const belongsToTag = targetTag === ALL_TAGS
      || (targetTag === UNTAGGED ? articleTagIds.length === 0 : articleTagIds.includes(targetTag))
    return belongsToTag
      && !readIds.has(article.id)
      && !savedIds.has(article.id)
      && !deletedIds.has(article.id)
  }).length

  const remainingSourceCount = (targetSource: string) => articles.filter((article) => (
    (targetSource === ALL_SOURCES || article.sourceId === targetSource)
      && !readIds.has(article.id)
      && !savedIds.has(article.id)
      && !deletedIds.has(article.id)
  )).length

  const activeFilterName = filterMode === 'source'
    ? sourceOptions.find((item) => item.id === source)?.name
    : tagOptions.find((item) => item.id === tag)?.name

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="#/" aria-label="fliqrss ホーム">
          <span className="brand-mark"><span /></span>
          <span>fliq<span>rss</span></span>
        </a>

        <div className="topbar-status">
          <span className="live-dot" />
          <span>{loading ? 'API接続中' : apiError ? 'APIエラー' : 'API接続済み'}</span>
          <span className="status-divider" />
          <span>{articles.length} stories</span>
        </div>

        <div className="header-menu">
          <button
            aria-controls="main-menu"
            aria-expanded={menuOpen}
            aria-label={menuOpen ? 'メニューを閉じる' : 'メニューを開く'}
            className={`menu-toggle ${menuOpen ? 'is-open' : ''}`}
            onClick={() => setMenuOpen((current) => !current)}
            type="button"
          >
            <Icon name={menuOpen ? 'close' : 'menu'} size={22} />
          </button>
          {menuOpen && (
            <>
              <button aria-label="メニューを閉じる" className="menu-scrim" onClick={() => setMenuOpen(false)} type="button" />
              <nav aria-label="メインメニュー" className="main-menu" id="main-menu">
                <button className={sourceManagerOpen ? 'is-active' : ''} onClick={navigateToSources} type="button">
                  <Icon name="rss" size={19} />
                  <span>ソース</span>
                  <strong>{managedSources.length}</strong>
                </button>
                <button className={tagManagerOpen ? 'is-active' : ''} onClick={navigateToTags} type="button">
                  <Icon name="tag" size={19} />
                  <span>タグ</span>
                  <strong>{managedTags.length}</strong>
                </button>
                <span className="main-menu-divider" />
                <button className={libraryMode === 'favorite' ? 'is-active' : ''} onClick={() => navigateToLibrary('favorite')} type="button">
                  <Icon name="star" size={19} />
                  <span>お気に入り</span>
                  <strong>{favoriteIds.size}</strong>
                </button>
                <button className={libraryMode === 'saved' ? 'is-active' : ''} onClick={() => navigateToLibrary('saved')} type="button">
                  <Icon name="bookmark" size={19} />
                  <span>あとで見る</span>
                  <strong>{savedIds.size}</strong>
                </button>
                <button className={libraryMode === 'deleted' ? 'is-active' : ''} onClick={() => navigateToLibrary('deleted')} type="button">
                  <Icon name="trash" size={19} />
                  <span>ゴミ箱</span>
                  <strong>{deletedIds.size}</strong>
                </button>
              </nav>
            </>
          )}
        </div>
      </header>

      {apiError && (
        <div className="api-error" role="alert">
          <span>{apiError}</span>
          <button onClick={() => void loadData()} type="button">再試行</button>
        </div>
      )}

      {sourceManagerOpen ? (
        <main className="category-page" id="top">
          <div className="library-page-heading">
            <button className="back-to-reader" onClick={navigateToReader} type="button">
              <Icon name="arrow-left" size={19} /> 戻る
            </button>
            <p className="eyebrow">SOURCE SETTINGS</p>
            <h1>ニュースソース</h1>
            <p className="category-page-description">
              収集するRSS・Atomを追加し, 取得状態とタグの割り当てをニュースソースごとに管理できます.
            </p>
          </div>

          <form className="source-add-form" onSubmit={addSource}>
            <div className="source-form-field source-form-field--name">
              <label htmlFor="new-source-name">ソース名</label>
              <input
                id="new-source-name"
                maxLength={60}
                onChange={(event) => setNewSourceName(event.target.value)}
                placeholder="例: Example News"
                required
                type="text"
                value={newSourceName}
              />
            </div>
            <div className="source-form-field source-form-field--url">
              <label htmlFor="new-source-url">フィードURL</label>
              <input
                id="new-source-url"
                onChange={(event) => setNewSourceUrl(event.target.value)}
                placeholder="https://example.com/feed.xml"
                required
                type="url"
                value={newSourceUrl}
              />
            </div>
            <button className="source-add-button" disabled={pending || !newSourceName.trim() || !newSourceUrl.trim()} type="submit">
              {pending ? '取得中' : '追加'}
            </button>
          </form>

          <section className="source-manager-list" aria-label="ニュースソース一覧">
            {managedSources.map((item) => (
              <article className="source-manager-item" key={item.id}>
                <div className="source-manager-icon"><Icon name="rss" size={20} /></div>
                <div className="source-manager-details">
                  <div>
                    <strong>{item.name}</strong>
                    <span className={`source-status ${item.enabled ? 'is-enabled' : ''}`}>
                      {item.enabled ? '取得中' : '停止中'}
                    </span>
                    <span className="source-format">{item.format.toUpperCase()}</span>
                  </div>
                  <a href={item.url} rel="noreferrer" target="_blank">{item.url}</a>
                  <div className="source-tag-control">
                    <div className="selected-source-tags" aria-label={`${item.name}に設定中のタグ`}>
                      {tagNamesForSource(item.id).map((tagName) => <span key={tagName}>{tagName}</span>)}
                      {!tagNamesForSource(item.id).length && <em>タグなし</em>}
                    </div>
                    <button
                      aria-expanded={openTagPickerSourceId === item.id}
                      aria-label={`${item.name}のタグを設定`}
                      className="tag-picker-toggle"
                      onClick={() => setOpenTagPickerSourceId((current) => current === item.id ? null : item.id)}
                      type="button"
                    >
                      +
                    </button>
                    {openTagPickerSourceId === item.id && (
                      <>
                        <button aria-label="タグ一覧を閉じる" className="tag-picker-scrim" onClick={() => setOpenTagPickerSourceId(null)} type="button" />
                        <div className="tag-picker" role="dialog" aria-label={`${item.name}へ割り当てるタグ`}>
                          <div className="tag-picker-heading">
                            <strong>タグを選択</strong>
                            <button aria-label="閉じる" onClick={() => setOpenTagPickerSourceId(null)} type="button">×</button>
                          </div>
                          <div className="tag-picker-list">
                            {managedTags.map((tagItem) => {
                              const selected = tagIdsForSource(item.id).includes(tagItem.id)
                              return (
                                <button
                                  aria-pressed={selected}
                                  className={selected ? 'is-active' : ''}
                                  key={tagItem.id}
                                  onClick={() => void toggleSourceTag(item, tagItem.id)}
                                  type="button"
                                >
                                  <span>{selected ? '✓' : ''}</span>
                                  {tagItem.name}
                                </button>
                              )
                            })}
                            {!managedTags.length && <p>設定可能なタグがありません.</p>}
                          </div>
                        </div>
                      </>
                    )}
                  </div>
                </div>
                <div className="source-manager-actions">
                  <button onClick={() => void toggleSourceEnabled(item)} type="button">
                    {item.enabled ? '停止' : '再開'}
                  </button>
                  <button className="category-delete-button" onClick={() => deleteSource(item)} type="button">削除</button>
                </div>
              </article>
            ))}
            {!managedSources.length && <div className="library-empty">ニュースソースがありません.</div>}
          </section>
          <p className="source-mock-note">形式は取得したXMLの内容から自動判定します.</p>
        </main>
      ) : tagManagerOpen ? (
        <main className="category-page" id="top">
          <div className="library-page-heading">
            <button className="back-to-reader" onClick={navigateToReader} type="button">
              <Icon name="arrow-left" size={19} /> 戻る
            </button>
            <p className="eyebrow">TAG SETTINGS</p>
            <h1>タグ一覧</h1>
            <p className="category-page-description">
              ニュースソースに設定可能なタグの追加, 名前変更, 削除ができます. タグの割り当てはソースページで行います.
            </p>
          </div>

          <form className="category-add-form" onSubmit={addTag}>
            <label htmlFor="new-tag">新しいタグ</label>
            <div>
              <input
                id="new-tag"
                maxLength={30}
                onChange={(event) => setNewTagName(event.target.value)}
                placeholder="例: デザイン"
                type="text"
                value={newTagName}
              />
              <button disabled={!newTagName.trim()} type="submit">追加</button>
            </div>
          </form>

          <section className="category-manager-list" aria-label="タグ一覧">
            <div className="category-manager-header">
              <span>タグ名</span>
              <span>ソース数</span>
              <span>操作</span>
            </div>
            {managedTags.map((item) => {
              const sourceCount = managedSources.filter((sourceItem) => tagIdsForSource(sourceItem.id).includes(item.id)).length
              return (
                <div className="category-manager-item" key={item.id}>
                  {editingTagId === item.id ? (
                    <form className="category-edit-form" onSubmit={saveTagName}>
                      <input
                        aria-label="タグ名"
                        autoFocus
                        maxLength={30}
                        onChange={(event) => setEditingTagName(event.target.value)}
                        value={editingTagName}
                      />
                      <button disabled={!editingTagName.trim()} type="submit">保存</button>
                      <button onClick={() => setEditingTagId(null)} type="button">取消</button>
                    </form>
                  ) : (
                    <strong>{item.name}</strong>
                  )}
                  <span className="category-article-count">{sourceCount}件</span>
                  <div className="category-row-actions">
                    <button onClick={() => startEditingTag(item)} type="button">変更</button>
                    <button className="category-delete-button" onClick={() => deleteTag(item.id)} type="button">削除</button>
                  </div>
                </div>
              )
            })}
            {!managedTags.length && <div className="library-empty">タグがありません.</div>}
          </section>

        </main>
      ) : libraryMode ? (
        <main className="library-page" id="top">
          <div className="library-page-heading">
            <button className="back-to-reader" onClick={navigateToReader} type="button">
              <Icon name="arrow-left" size={19} /> 戻る
            </button>
            <p className="eyebrow">YOUR LIBRARY</p>
            <h1>記事ライブラリ</h1>
          </div>
          <div className="library-tabs" role="tablist" aria-label="記事の状態">
            <button className={libraryMode === 'favorite' ? 'is-active' : ''} onClick={() => navigateToLibrary('favorite')} type="button">
              お気に入り <span>{favoriteIds.size}</span>
            </button>
            <button className={libraryMode === 'saved' ? 'is-active' : ''} onClick={() => navigateToLibrary('saved')} type="button">
              あとで見る <span>{savedIds.size}</span>
            </button>
            <button className={libraryMode === 'deleted' ? 'is-active' : ''} onClick={() => navigateToLibrary('deleted')} type="button">
              ゴミ箱 <span>{deletedIds.size}</span>
            </button>
          </div>
          <div className="library-list">
            {libraryArticles.map((article) => (
              <article className="library-item" key={article.id}>
                <div className={`queue-thumb queue-thumb--${article.visualTheme}`}>{article.sourceInitials}</div>
                <button className="library-article-title" onClick={() => openDetail(article)} type="button">
                  <span>{tagNamesForSource(article.sourceId).join(' / ') || 'タグなし'} · {article.source}</span>
                  <strong>{article.title}</strong>
                </button>
                <button className="library-remove" onClick={() => removeFromLibrary(article.id)} type="button">
                  {libraryMode === 'deleted' ? '復元' : '解除'}
                </button>
              </article>
            ))}
            {!libraryArticles.length && <div className="library-empty">該当する記事はありません.</div>}
          </div>
        </main>
      ) : (
        <main className="workspace" id="top">
        <aside className="feed-panel">
          <p className="eyebrow">YOUR DAILY FLOW</p>
          <h1>ニュースを,<br />軽やかに.</h1>
          <p className="intro-copy">保存は右へ. 既読は左へ. 削除はカード左上から. あなたのテンポで情報をめくろう.</p>

          <div className="filter-mode-switch" aria-label="絞り込み方法" role="tablist">
            <button
              aria-selected={filterMode === 'source'}
              className={filterMode === 'source' ? 'is-active' : ''}
              onClick={() => selectFilterMode('source')}
              role="tab"
              type="button"
            >
              ニュースソース
            </button>
            <button
              aria-selected={filterMode === 'tag'}
              className={filterMode === 'tag' ? 'is-active' : ''}
              onClick={() => selectFilterMode('tag')}
              role="tab"
              type="button"
            >
              タグ
            </button>
          </div>

          <nav aria-label={filterMode === 'source' ? 'ニュースソース' : 'タグ'} className="category-nav filter-option-list">
            {(filterMode === 'source' ? sourceOptions : tagOptions).map((item) => (
              <button
                aria-current={(filterMode === 'source' ? source : tag) === item.id ? 'page' : undefined}
                className={(filterMode === 'source' ? source : tag) === item.id ? 'is-active' : ''}
                key={item.id}
                onClick={() => filterMode === 'source' ? selectSource(item.id) : selectTag(item.id)}
                type="button"
              >
                <span>{item.name}</span>
                <span>
                  {filterMode === 'source' ? remainingSourceCount(item.id) : remainingTagCount(item.id)}
                </span>
              </button>
            ))}
          </nav>

          <div className="keyboard-help">
            <span><kbd>←</kbd> スキップ</span>
            <span><kbd>→</kbd> 保存</span>
          </div>
        </aside>

        <section className="reader-panel" aria-live="polite">
          <div className="mobile-filter-mode" aria-label="絞り込み方法">
            <button className={filterMode === 'source' ? 'is-active' : ''} onClick={() => selectFilterMode('source')} type="button">
              ニュースソース
            </button>
            <button className={filterMode === 'tag' ? 'is-active' : ''} onClick={() => selectFilterMode('tag')} type="button">
              タグ
            </button>
          </div>
          <div className="mobile-categories" aria-label={filterMode === 'source' ? 'ニュースソース' : 'タグ'}>
            {(filterMode === 'source' ? sourceOptions : tagOptions).map((item) => (
              <button
                className={(filterMode === 'source' ? source : tag) === item.id ? 'is-active' : ''}
                key={item.id}
                onClick={() => filterMode === 'source' ? selectSource(item.id) : selectTag(item.id)}
                type="button"
              >
                {item.name}
              </button>
            ))}
          </div>

          <div className="reader-heading">
            <span>{filterMode === 'source' ? 'ニュースソース' : 'タグ'} · {activeFilterName ?? 'すべて'}</span>
            <div className="progress-count">
              <strong>
                {String(currentArticle ? processedCount + 1 : selectionArticles.length).padStart(2, '0')}
              </strong>
              <span>/ {String(selectionArticles.length).padStart(2, '0')}</span>
            </div>
          </div>

          <div className="progress-track"><span style={{ width: `${progress}%` }} /></div>

          <div className="card-stage">
            {loading ? (
              <div className="queue-complete">
                <p className="eyebrow">CONNECTING</p>
                <h2>記事を取得しています</h2>
              </div>
            ) : currentArticle ? (
              <>
                <div className="stack-card stack-card--back" />
                <div className="stack-card stack-card--middle" />
                <ArticleCard
                  article={currentArticle}
                  tagLabels={tagNamesForSource(currentArticle.sourceId)}
                  dragX={dragX}
                  dragging={dragging}
                  isFavorite={favoriteIds.has(currentArticle.id)}
                  onDelete={() => deleteArticle(currentArticle.id)}
                  onOpen={() => openDetail(currentArticle)}
                  onToggleFavorite={() => toggleFavorite(currentArticle.id)}
                  onPointerCancel={handlePointerCancel}
                  onPointerDown={handlePointerDown}
                  onPointerMove={handlePointerMove}
                  onPointerUp={handlePointerUp}
                />
              </>
            ) : (
              <div className="queue-complete">
                <div className="complete-mark"><Icon name="check" size={34} /></div>
                <p className="eyebrow">YOU ARE ALL CAUGHT UP</p>
                <h2>表示できる未読記事はありません</h2>
                {skippedIds.size > 0 && (
                  <button type="button" onClick={restoreSkippedArticles}>
                    <Icon name="refresh" size={18} /> スキップ解除
                  </button>
                )}
              </div>
            )}
          </div>

          <div className="reader-actions">
            <button
              className="action-button action-button--skip"
              disabled={!currentArticle || animating}
              onClick={() => completeSwipe('skip')}
              type="button"
            >
              <Icon name="close" size={22} />
              <span>スキップ</span>
            </button>
            <button
              className="action-button action-button--save"
              disabled={!currentArticle || animating}
              onClick={() => completeSwipe('save')}
              type="button"
            >
              <Icon name="bookmark" size={20} />
              <span>保存</span>
            </button>
          </div>
        </section>

        </main>
      )}

      {notice && <div className="notice" role="status"><Icon name="check" size={17} />{notice}</div>}

      {openArticle && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setOpenArticle(null)}>
          <article
            aria-labelledby="detail-title"
            aria-modal="true"
            className="article-modal"
            onMouseDown={(event) => event.stopPropagation()}
            role="dialog"
          >
            <button className="modal-close" onClick={() => setOpenArticle(null)} type="button" aria-label="閉じる">
              <Icon name="close" size={22} />
            </button>
            <div className={`modal-visual article-visual--${openArticle.visualTheme}`}>
              <span>{openArticle.visualLabel}</span>
            </div>
            <div className="modal-content">
              <div className="article-meta">
                <span className="source-mark">{openArticle.sourceInitials}</span>
                <span className="source-name">{openArticle.source}</span>
                <span className="meta-dot" />
                <span>{openArticle.publishedAt}</span>
              </div>
              <div className="article-tags modal-tags">
                {(tagNamesForSource(openArticle.sourceId).length ? tagNamesForSource(openArticle.sourceId) : ['タグなし']).map(
                  (tagName) => <span key={tagName}>{tagName}</span>,
                )}
              </div>
              <h2 id="detail-title">{openArticle.title}</h2>
              {openArticle.body.map((paragraph) => <p key={paragraph}>{paragraph}</p>)}
              {openArticle.url && (
                <a className="mock-note" href={openArticle.url} rel="noreferrer" target="_blank">元の記事を開く</a>
              )}
              <button
                aria-pressed={favoriteIds.has(openArticle.id)}
                className={`modal-favorite ${favoriteIds.has(openArticle.id) ? 'is-active' : ''}`}
                onClick={() => toggleFavorite(openArticle.id)}
                type="button"
              >
                <Icon name="star" size={18} />
                {favoriteIds.has(openArticle.id) ? 'お気に入り解除' : 'お気に入り追加'}
              </button>
              <button
                className="modal-next"
                onClick={() => setOpenArticle(null)}
                type="button"
              >
                {libraryMode ? '一覧へ' : 'カードへ'} <Icon name="chevron-right" size={18} />
              </button>
            </div>
          </article>
        </div>
      )}

    </div>
  )
}

export default App
