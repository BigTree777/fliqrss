import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArticleCard } from './components/ArticleCard'
import { Icon } from './components/Icon'
import { articles } from './mocks/articles'
import type { Article } from './types/article'

interface ManagedTag {
  id: string
  name: string
}

interface ManagedSource {
  id: string
  name: string
  url: string
  format: 'RSS' | 'Atom'
  enabled: boolean
}

type TagFilter = string
type FilterMode = 'source' | 'tag'
type SwipeAction = 'skip' | 'save'
type LibraryMode = 'favorite' | 'saved' | 'deleted'

const ALL_TAGS = '__all__'
const UNTAGGED = '__untagged__'
const ALL_SOURCES = '__all_sources__'
const defaultTags: ManagedTag[] = [
  { id: 'technology', name: 'テクノロジー' },
  { id: 'business', name: 'ビジネス' },
  { id: 'culture', name: 'カルチャー' },
  { id: 'science', name: 'サイエンス' },
]
const defaultSourceTags: Record<string, string[]> = {
  'Orbit Journal': ['technology', 'science'],
  'Business Field': ['business'],
  'Nook Magazine': ['culture'],
  'Scope Science': ['science', 'technology'],
  'Common Ledger': ['business', 'culture'],
  'Open Current': ['technology', 'science'],
}
const defaultSources: ManagedSource[] = [
  { id: 'orbit-journal', name: 'Orbit Journal', url: 'https://example.com/orbit/rss.xml', format: 'RSS', enabled: true },
  { id: 'business-field', name: 'Business Field', url: 'https://example.com/business/atom.xml', format: 'Atom', enabled: true },
  { id: 'nook-magazine', name: 'Nook Magazine', url: 'https://example.com/nook/feed.xml', format: 'RSS', enabled: true },
  { id: 'scope-science', name: 'Scope Science', url: 'https://example.com/scope/atom.xml', format: 'Atom', enabled: true },
  { id: 'common-ledger', name: 'Common Ledger', url: 'https://example.com/ledger/rss.xml', format: 'RSS', enabled: true },
  { id: 'open-current', name: 'Open Current', url: 'https://example.com/current/feed.xml', format: 'RSS', enabled: true },
]
const SWIPE_THRESHOLD = 92
const STORAGE_KEYS = {
  deleted: 'fliqrss:deleted',
  favorite: 'fliqrss:favorite',
  read: 'fliqrss:read',
  saved: 'fliqrss:saved',
  skipped: 'fliqrss:skipped',
  sources: 'fliqrss:sources',
  sourceTags: 'fliqrss:source-tags',
  tags: 'fliqrss:tags',
} as const

function readStoredTags(): ManagedTag[] {
  try {
    const stored = JSON.parse(window.localStorage.getItem(STORAGE_KEYS.tags) ?? 'null')
    if (!Array.isArray(stored)) return defaultTags
    return stored.filter((item): item is ManagedTag => (
      typeof item === 'object'
      && item !== null
      && typeof item.id === 'string'
      && typeof item.name === 'string'
      && item.name.trim().length > 0
    ))
  } catch {
    return defaultTags
  }
}

function readStoredSourceTags(): Record<string, string[]> {
  try {
    const stored: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEYS.sourceTags) ?? 'null')
    if (!stored || typeof stored !== 'object' || Array.isArray(stored)) return defaultSourceTags
    return Object.fromEntries(Object.entries(stored).map(([source, tagIds]) => [
      source,
      Array.isArray(tagIds) ? tagIds.filter((id): id is string => typeof id === 'string') : [],
    ]))
  } catch {
    return defaultSourceTags
  }
}

function readStoredSources(): ManagedSource[] {
  try {
    const stored = JSON.parse(window.localStorage.getItem(STORAGE_KEYS.sources) ?? 'null')
    if (!Array.isArray(stored)) return defaultSources
    return stored.filter((item): item is ManagedSource => (
      typeof item === 'object'
      && item !== null
      && typeof item.id === 'string'
      && typeof item.name === 'string'
      && typeof item.url === 'string'
      && (item.format === 'RSS' || item.format === 'Atom')
      && typeof item.enabled === 'boolean'
    ))
  } catch {
    return defaultSources
  }
}

function detectMockFeedFormat(url: string): ManagedSource['format'] {
  return url.toLocaleLowerCase().includes('atom') ? 'Atom' : 'RSS'
}

function readStoredIds(key: string): Set<string> {
  try {
    const stored = JSON.parse(window.localStorage.getItem(key) ?? '[]')
    return new Set(Array.isArray(stored) ? stored.filter((id): id is string => typeof id === 'string') : [])
  } catch {
    return new Set()
  }
}

function libraryModeFromHash(): LibraryMode | null {
  if (window.location.hash === '#/favorites') return 'favorite'
  if (window.location.hash === '#/saved') return 'saved'
  if (window.location.hash === '#/deleted') return 'deleted'
  return null
}

function App() {
  const [filterMode, setFilterMode] = useState<FilterMode>('source')
  const [source, setSource] = useState(ALL_SOURCES)
  const [managedSources, setManagedSources] = useState<ManagedSource[]>(readStoredSources)
  const [sourceManagerOpen, setSourceManagerOpen] = useState(() => window.location.hash === '#/sources')
  const [newSourceName, setNewSourceName] = useState('')
  const [newSourceUrl, setNewSourceUrl] = useState('')
  const [openTagPickerSourceId, setOpenTagPickerSourceId] = useState<string | null>(null)
  const [tag, setTag] = useState<TagFilter>(ALL_TAGS)
  const [managedTags, setManagedTags] = useState<ManagedTag[]>(readStoredTags)
  const [sourceTags, setSourceTags] = useState<Record<string, string[]>>(readStoredSourceTags)
  const [tagManagerOpen, setTagManagerOpen] = useState(() => window.location.hash === '#/tags')
  const [menuOpen, setMenuOpen] = useState(false)
  const [newTagName, setNewTagName] = useState('')
  const [editingTagId, setEditingTagId] = useState<string | null>(null)
  const [editingTagName, setEditingTagName] = useState('')
  const [dragX, setDragX] = useState(0)
  const [dragging, setDragging] = useState(false)
  const [animating, setAnimating] = useState(false)
  const [deletedIds, setDeletedIds] = useState<Set<string>>(() => readStoredIds(STORAGE_KEYS.deleted))
  const [favoriteIds, setFavoriteIds] = useState<Set<string>>(() => readStoredIds(STORAGE_KEYS.favorite))
  const [savedIds, setSavedIds] = useState<Set<string>>(() => readStoredIds(STORAGE_KEYS.saved))
  const [readIds, setReadIds] = useState<Set<string>>(() => readStoredIds(STORAGE_KEYS.read))
  const [skippedIds, setSkippedIds] = useState<Set<string>>(() => readStoredIds(STORAGE_KEYS.skipped))
  const [openArticle, setOpenArticle] = useState<Article | null>(null)
  const [libraryMode, setLibraryMode] = useState<LibraryMode | null>(() => libraryModeFromHash())
  const [notice, setNotice] = useState('')
  const pointerStart = useRef(0)
  const activePointer = useRef<number | null>(null)
  const animationTimer = useRef<number | null>(null)
  const noticeTimer = useRef<number | null>(null)

  const knownTagIds = useMemo(() => new Set(managedTags.map((item) => item.id)), [managedTags])
  const tagIdsForSource = useCallback((source: string) => (
    (sourceTags[source] ?? []).filter((tagId) => knownTagIds.has(tagId))
  ), [knownTagIds, sourceTags])
  const selectionArticles = useMemo(() => {
    if (filterMode === 'source') {
      return source === ALL_SOURCES ? articles : articles.filter((article) => article.source === source)
    }
    if (tag === ALL_TAGS) return articles
    if (tag === UNTAGGED) return articles.filter((article) => tagIdsForSource(article.source).length === 0)
    return articles.filter((article) => tagIdsForSource(article.source).includes(tag))
  }, [filterMode, source, tag, tagIdsForSource])

  const untaggedCount = useMemo(
    () => articles.filter((article) => tagIdsForSource(article.source).length === 0).length,
    [tagIdsForSource],
  )

  const tagNamesForSource = useCallback((source: string) => tagIdsForSource(source).map(
    (tagId) => managedTags.find((item) => item.id === tagId)?.name,
  ).filter((name): name is string => Boolean(name)), [managedTags, tagIdsForSource])

  const tagOptions = useMemo(() => [
    { id: ALL_TAGS, name: 'すべて' },
    ...managedTags,
    ...(untaggedCount ? [{ id: UNTAGGED, name: 'タグなし' }] : []),
  ], [managedTags, untaggedCount])

  const sourceOptions = useMemo(() => [
    { id: ALL_SOURCES, name: 'すべてのソース' },
    ...managedSources.map((item) => ({ id: item.name, name: item.name })),
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

  const addSource = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = newSourceName.trim()
    const url = newSourceUrl.trim()
    if (!name || !url) return
    if (managedSources.some((item) => item.url.toLocaleLowerCase() === url.toLocaleLowerCase())) {
      showNotice('同じURLのソースがあります')
      return
    }
    setManagedSources((current) => [...current, {
      id: `source-${Date.now()}`,
      name,
      url,
      format: detectMockFeedFormat(url),
      enabled: true,
    }])
    setNewSourceName('')
    setNewSourceUrl('')
    showNotice('ニュースソースを追加しました')
  }

  const toggleSourceEnabled = (sourceId: string) => {
    setManagedSources((current) => current.map((item) => (
      item.id === sourceId ? { ...item, enabled: !item.enabled } : item
    )))
  }

  const deleteSource = (targetSource: ManagedSource) => {
    setManagedSources((current) => current.filter((item) => item.id !== targetSource.id))
    setSourceTags((current) => {
      const next = { ...current }
      delete next[targetSource.name]
      return next
    })
    if (source === targetSource.name) setSource(ALL_SOURCES)
    if (openTagPickerSourceId === targetSource.id) setOpenTagPickerSourceId(null)
    showNotice('ニュースソースを削除しました')
  }

  const addTag = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = newTagName.trim()
    if (!name) return
    if (managedTags.some((item) => item.name.toLocaleLowerCase() === name.toLocaleLowerCase())) {
      showNotice('同じ名前のタグがあります')
      return
    }
    setManagedTags((current) => [...current, { id: `custom-${Date.now()}`, name }])
    setNewTagName('')
    showNotice('タグを追加しました')
  }

  const startEditingTag = (item: ManagedTag) => {
    setEditingTagId(item.id)
    setEditingTagName(item.name)
  }

  const saveTagName = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = editingTagName.trim()
    if (!editingTagId || !name) return
    if (managedTags.some((item) => item.id !== editingTagId && item.name.toLocaleLowerCase() === name.toLocaleLowerCase())) {
      showNotice('同じ名前のタグがあります')
      return
    }
    setManagedTags((current) => current.map((item) => (
      item.id === editingTagId ? { ...item, name } : item
    )))
    setEditingTagId(null)
    showNotice('タグ名を変更しました')
  }

  const deleteTag = (tagId: string) => {
    setManagedTags((current) => current.filter((item) => item.id !== tagId))
    setSourceTags((current) => Object.fromEntries(Object.entries(current).map(([source, tagIds]) => [
      source,
      tagIds.filter((id) => id !== tagId),
    ])))
    if (tag === tagId) setTag(ALL_TAGS)
    if (editingTagId === tagId) setEditingTagId(null)
    showNotice('タグを削除しました')
  }

  const toggleSourceTag = (source: string, tagId: string) => {
    setSourceTags((current) => {
      const currentTagIds = current[source] ?? []
      const nextTagIds = currentTagIds.includes(tagId)
        ? currentTagIds.filter((id) => id !== tagId)
        : [...currentTagIds, tagId]
      return { ...current, [source]: nextTagIds }
    })
  }

  const completeSwipe = useCallback(
    (action: SwipeAction) => {
      if (!currentArticle || animating) return

      setAnimating(true)
      setDragging(false)
      const direction = action === 'save' ? 1 : -1
      setDragX(direction * Math.max(window.innerWidth * 0.7, 680))
      showNotice(action === 'save' ? 'あとで読むに保存しました' : '既読にしました')

      animationTimer.current = window.setTimeout(() => {
        if (action === 'save') {
          setSavedIds((current) => new Set(current).add(currentArticle.id))
        } else if (action === 'skip') {
          setReadIds((current) => new Set(current).add(currentArticle.id))
          setSkippedIds((current) => new Set(current).add(currentArticle.id))
        }
        setDragX(0)
        setAnimating(false)
      }, 260)
    },
    [animating, currentArticle, showNotice],
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

  const openDetail = (article: Article) => {
    setReadIds((current) => new Set(current).add(article.id))
    setOpenArticle(article)
  }

  const toggleFavorite = (articleId: string) => {
    setFavoriteIds((current) => {
      const next = new Set(current)
      if (next.has(articleId)) {
        next.delete(articleId)
        showNotice('お気に入りから外しました')
      } else {
        next.add(articleId)
        showNotice('お気に入りに追加しました')
      }
      return next
    })
  }

  const deleteArticle = (articleId: string) => {
    setDeletedIds((current) => new Set(current).add(articleId))
    setFavoriteIds((current) => {
      const next = new Set(current)
      next.delete(articleId)
      return next
    })
    showNotice('削除記事一覧へ移動しました')
  }

  const removeFromLibrary = (articleId: string) => {
    if (libraryMode === 'favorite') {
      setFavoriteIds((current) => {
        const next = new Set(current)
        next.delete(articleId)
        return next
      })
      showNotice('お気に入りから外しました')
    } else if (libraryMode === 'saved') {
      setSavedIds((current) => {
        const next = new Set(current)
        next.delete(articleId)
        return next
      })
      showNotice('保存を解除しました')
    } else if (libraryMode === 'deleted') {
      setDeletedIds((current) => {
        const next = new Set(current)
        next.delete(articleId)
        return next
      })
      showNotice('削除記事一覧から戻しました')
    }
  }

  const restoreSkippedArticles = () => {
    setReadIds((current) => {
      const next = new Set(current)
      skippedIds.forEach((id) => next.delete(id))
      return next
    })
    setSkippedIds(new Set())
    setDragX(0)
    showNotice('スキップした記事を戻しました')
  }

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.sources, JSON.stringify(managedSources))
  }, [managedSources])

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.tags, JSON.stringify(managedTags))
  }, [managedTags])

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.sourceTags, JSON.stringify(sourceTags))
  }, [sourceTags])

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.deleted, JSON.stringify([...deletedIds]))
  }, [deletedIds])

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.favorite, JSON.stringify([...favoriteIds]))
  }, [favoriteIds])

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.saved, JSON.stringify([...savedIds]))
  }, [savedIds])

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.read, JSON.stringify([...readIds]))
  }, [readIds])

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEYS.skipped, JSON.stringify([...skippedIds]))
  }, [skippedIds])

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
    const articleTagIds = tagIdsForSource(article.source)
    const belongsToTag = targetTag === ALL_TAGS
      || (targetTag === UNTAGGED ? articleTagIds.length === 0 : articleTagIds.includes(targetTag))
    return belongsToTag
      && !readIds.has(article.id)
      && !savedIds.has(article.id)
      && !deletedIds.has(article.id)
  }).length

  const remainingSourceCount = (targetSource: string) => articles.filter((article) => (
    (targetSource === ALL_SOURCES || article.source === targetSource)
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
          <span>モックフィード</span>
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
            <button className="source-add-button" disabled={!newSourceName.trim() || !newSourceUrl.trim()} type="submit">追加</button>
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
                    <span className="source-format">{item.format}</span>
                  </div>
                  <a href={item.url} rel="noreferrer" target="_blank">{item.url}</a>
                  <div className="source-tag-control">
                    <div className="selected-source-tags" aria-label={`${item.name}に設定中のタグ`}>
                      {tagNamesForSource(item.name).map((tagName) => <span key={tagName}>{tagName}</span>)}
                      {!tagNamesForSource(item.name).length && <em>タグなし</em>}
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
                              const selected = tagIdsForSource(item.name).includes(tagItem.id)
                              return (
                                <button
                                  aria-pressed={selected}
                                  className={selected ? 'is-active' : ''}
                                  key={tagItem.id}
                                  onClick={() => toggleSourceTag(item.name, tagItem.id)}
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
                  <button onClick={() => toggleSourceEnabled(item.id)} type="button">
                    {item.enabled ? '停止' : '再開'}
                  </button>
                  <button className="category-delete-button" onClick={() => deleteSource(item)} type="button">削除</button>
                </div>
              </article>
            ))}
            {!managedSources.length && <div className="library-empty">ニュースソースがありません.</div>}
          </section>
          <p className="source-mock-note">形式は自動判定します. 現在はURLによる簡易判定で, バックエンド接続後は取得したXMLの内容から判定します.</p>
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
              const sourceCount = managedSources.filter((sourceItem) => tagIdsForSource(sourceItem.name).includes(item.id)).length
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
                  <span>{tagNamesForSource(article.source).join(' / ') || 'タグなし'} · {article.source}</span>
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
            {currentArticle ? (
              <>
                <div className="stack-card stack-card--back" />
                <div className="stack-card stack-card--middle" />
                <ArticleCard
                  article={currentArticle}
                  tagLabels={tagNamesForSource(currentArticle.source)}
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
                {(tagNamesForSource(openArticle.source).length ? tagNamesForSource(openArticle.source) : ['タグなし']).map(
                  (tagName) => <span key={tagName}>{tagName}</span>,
                )}
              </div>
              <h2 id="detail-title">{openArticle.title}</h2>
              {openArticle.body.map((paragraph) => <p key={paragraph}>{paragraph}</p>)}
              <div className="mock-note">これはUI確認用のダミー記事です.</div>
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
