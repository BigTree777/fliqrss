import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, ApiError } from './api/client'
import type { OPMLImportResult } from './api/client'
import { ArticleCard } from './components/ArticleCard'
import { Icon } from './components/Icon'
import type { Article, ArticleAction, ArticleStats, Source, Tag } from './types/article'

type TagFilter = string
type FilterMode = 'source' | 'tag'
type SwipeAction = 'skip' | 'save'
type LibraryMode = 'favorite' | 'saved' | 'deleted'
type ColorTheme = 'light' | 'dark'

const ALL_TAGS = '__all__'
const UNTAGGED = '__untagged__'
const ALL_SOURCES = '__all_sources__'
const SWIPE_THRESHOLD = 92
const FEED_PAGE_SIZE = 20
const PREFETCH_THRESHOLD = 5
const LIBRARY_PAGE_SIZE = 50
const THEME_STORAGE_KEY = 'fliqrss.theme'

function storedTheme(): ColorTheme | null {
  try {
    const value = window.localStorage.getItem(THEME_STORAGE_KEY)
    return value === 'light' || value === 'dark' ? value : null
  } catch {
    return null
  }
}

function initialTheme(): ColorTheme {
  if (document.documentElement.dataset.theme === 'dark') return 'dark'
  return 'light'
}

const emptyArticleStats: ArticleStats = {
  feed: 0,
  favorite: 0,
  saved: 0,
  deleted: 0,
  skipped: 0,
  untaggedFeed: 0,
  sourceFeedCounts: {},
  tagFeedCounts: {},
}

function libraryModeFromHash(): LibraryMode | null {
  if (window.location.hash === '#/favorites') return 'favorite'
  if (window.location.hash === '#/saved') return 'saved'
  if (window.location.hash === '#/deleted') return 'deleted'
  return null
}

function App() {
  const [theme, setTheme] = useState<ColorTheme>(initialTheme)
  const [filterMode, setFilterMode] = useState<FilterMode>('source')
  const [source, setSource] = useState(ALL_SOURCES)
  const [articles, setArticles] = useState<Article[]>([])
  const [nextCursor, setNextCursor] = useState<string | undefined>()
  const [feedTotal, setFeedTotal] = useState(0)
  const [processedCount, setProcessedCount] = useState(0)
  const [prefetching, setPrefetching] = useState(false)
  const [articleStats, setArticleStats] = useState<ArticleStats>(emptyArticleStats)
  const [libraryArticles, setLibraryArticles] = useState<Article[]>([])
  const [libraryNextCursor, setLibraryNextCursor] = useState<string | undefined>()
  const [libraryTotal, setLibraryTotal] = useState(0)
  const [libraryLoading, setLibraryLoading] = useState(false)
  const [managedSources, setManagedSources] = useState<Source[]>([])
  const [sourceManagerOpen, setSourceManagerOpen] = useState(() => window.location.hash === '#/sources')
  const [newSourceName, setNewSourceName] = useState('')
  const [newSourceUrl, setNewSourceUrl] = useState('')
  const [opmlFile, setOPMLFile] = useState<File | null>(null)
  const [opmlResult, setOPMLResult] = useState<OPMLImportResult | null>(null)
  const [importingOPML, setImportingOPML] = useState(false)
  const [exportingOPML, setExportingOPML] = useState(false)
  const [openTagPickerSourceId, setOpenTagPickerSourceId] = useState<string | null>(null)
  const [reorderingSourceId, setReorderingSourceId] = useState<string | null>(null)
  const [draggedSourceId, setDraggedSourceId] = useState<string | null>(null)
  const [dragOverSourceId, setDragOverSourceId] = useState<string | null>(null)
  const [refreshingSources, setRefreshingSources] = useState(false)
  const [markingAllRead, setMarkingAllRead] = useState(false)
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
  const [focusMode, setFocusMode] = useState(false)
  const [articleExpanded, setArticleExpanded] = useState(false)
  const [libraryMode, setLibraryMode] = useState<LibraryMode | null>(() => libraryModeFromHash())
  const [notice, setNotice] = useState('')
  const [apiError, setApiError] = useState('')
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState(false)
  const pointerStart = useRef({ x: 0, y: 0 })
  const pointerAxis = useRef<'horizontal' | 'vertical' | null>(null)
  const activePointer = useRef<number | null>(null)
  const animationTimer = useRef<number | null>(null)
  const noticeTimer = useRef<number | null>(null)
  const opmlInput = useRef<HTMLInputElement | null>(null)
  const feedRequestID = useRef(0)
  const libraryRequestID = useRef(0)

  useEffect(() => {
    const root = document.documentElement
    root.dataset.theme = theme
    root.style.colorScheme = theme
    document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.setAttribute(
      'content',
      theme === 'dark' ? '#101713' : '#f4f1e9',
    )
  }, [theme])

  useEffect(() => {
    if (storedTheme()) return
    const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
    const syncSystemTheme = (event: MediaQueryListEvent) => {
      if (!storedTheme()) setTheme(event.matches ? 'dark' : 'light')
    }
    systemTheme.addEventListener('change', syncSystemTheme)
    return () => systemTheme.removeEventListener('change', syncSystemTheme)
  }, [])

  const toggleTheme = () => {
    const nextTheme = theme === 'dark' ? 'light' : 'dark'
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, nextTheme)
    } catch {
      // The selected theme still applies for this session when storage is unavailable.
    }
    setTheme(nextTheme)
  }
  const tagIdsForSource = useCallback((sourceId: string) => (
    managedSources.find((item) => item.id === sourceId)?.tagIds ?? []
  ), [managedSources])
  const tagNamesForSource = useCallback((sourceId: string) => tagIdsForSource(sourceId).map(
    (tagId) => managedTags.find((item) => item.id === tagId)?.name,
  ).filter((name): name is string => Boolean(name)), [managedTags, tagIdsForSource])

  const tagOptions = useMemo(() => [
    { id: ALL_TAGS, name: 'すべて' },
    ...managedTags,
    ...(articleStats.untaggedFeed ? [{ id: UNTAGGED, name: 'タグなし' }] : []),
  ], [articleStats.untaggedFeed, managedTags])

  const sourceOptions = useMemo(() => [
    { id: ALL_SOURCES, name: 'すべてのソース' },
    ...managedSources.map((item) => ({ id: item.id, name: item.name })),
  ], [managedSources])

  const currentArticle = articles[0]

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

  const refreshStats = useCallback(async () => {
    const nextStats = await api.articleStats()
    setArticleStats(nextStats)
    return nextStats
  }, [])

  const feedPageQuery = useCallback((cursor?: string) => ({
    state: 'feed' as const,
    sourceId: filterMode === 'source' && source !== ALL_SOURCES ? source : undefined,
    tagId: filterMode === 'tag' && tag !== ALL_TAGS && tag !== UNTAGGED ? tag : undefined,
    untagged: filterMode === 'tag' && tag === UNTAGGED,
    cursor,
    limit: FEED_PAGE_SIZE,
  }), [filterMode, source, tag])

  const loadFeed = useCallback(async () => {
    const requestID = ++feedRequestID.current
    setLoading(true)
    setArticles([])
    setNextCursor(undefined)
    setProcessedCount(0)
    try {
      const page = await api.listArticlePage(feedPageQuery())
      if (requestID !== feedRequestID.current) return
      setArticles(page.items)
      setNextCursor(page.nextCursor)
      setFeedTotal(page.total)
      setApiError('')
    } catch (error) {
      if (requestID === feedRequestID.current) showApiError(error)
    } finally {
      if (requestID === feedRequestID.current) setLoading(false)
    }
  }, [feedPageQuery, showApiError])

  const loadData = useCallback(async () => {
    try {
      const [nextSources, nextTags, nextStats] = await Promise.all([
        api.listSources(),
        api.listTags(),
        api.articleStats(),
      ])
      setManagedSources(nextSources)
      setManagedTags(nextTags)
      setArticleStats(nextStats)
      setApiError('')
    } catch (error) {
      showApiError(error)
    }
  }, [showApiError])

  const replaceArticle = useCallback((nextArticle: Article) => {
    setArticles((current) => current.map((item) => item.id === nextArticle.id ? nextArticle : item))
    setLibraryArticles((current) => current.map((item) => item.id === nextArticle.id ? nextArticle : item))
  }, [])

  const updateArticleState = useCallback(async (articleId: string, action: ArticleAction) => {
    const nextArticle = await api.updateArticleState(articleId, action)
    replaceArticle(nextArticle)
    setApiError('')
    return nextArticle
  }, [replaceArticle])

  const navigateToLibrary = (mode: LibraryMode) => {
    setFocusMode(false)
    setArticleExpanded(false)
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
    setFocusMode(false)
    setArticleExpanded(false)
    setMenuOpen(false)
    setOpenTagPickerSourceId(null)
    window.location.hash = '#/'
    void loadFeed()
  }

  const navigateToTags = () => {
    setFocusMode(false)
    setArticleExpanded(false)
    setMenuOpen(false)
    setOpenTagPickerSourceId(null)
    window.location.hash = '#/tags'
  }

  const navigateToSources = () => {
    setFocusMode(false)
    setArticleExpanded(false)
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
      setManagedSources((current) => [...current, created])
      await Promise.all([loadFeed(), refreshStats()])
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

  const importOPML = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!opmlFile || importingOPML) return
    setImportingOPML(true)
    try {
      const result = await api.importOPML(opmlFile)
      const [nextSources, nextTags] = await Promise.all([
        api.listSources(),
        api.listTags(),
      ])
      setManagedSources(nextSources)
      setManagedTags(nextTags)
      await Promise.all([loadFeed(), refreshStats()])
      setOPMLResult(result)
      setOPMLFile(null)
      if (opmlInput.current) opmlInput.current.value = ''
      setApiError('')
      showNotice(`OPMLから${result.added}件追加しました`)
    } catch (error) {
      showApiError(error)
    } finally {
      setImportingOPML(false)
    }
  }

  const exportOPML = async () => {
    if (exportingOPML) return
    setExportingOPML(true)
    try {
      const blob = await api.exportOPML()
      const downloadURL = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = downloadURL
      link.download = 'fliqrss-subscriptions.opml'
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(downloadURL), 0)
      setApiError('')
      showNotice('OPMLを出力しました')
    } catch (error) {
      showApiError(error)
    } finally {
      setExportingOPML(false)
    }
  }

  const toggleSourceEnabled = async (targetSource: Source) => {
    try {
      const updated = await api.updateSource(targetSource.id, { enabled: !targetSource.enabled })
      setManagedSources((current) => current.map((item) => item.id === updated.id ? updated : item))
      await Promise.all([loadFeed(), refreshStats()])
      setApiError('')
    } catch (error) {
      showApiError(error)
    }
  }

  const deleteSource = async (targetSource: Source) => {
    try {
      await api.deleteSource(targetSource.id)
      setManagedSources((current) => current.filter((item) => item.id !== targetSource.id))
      if (source === targetSource.id) setSource(ALL_SOURCES)
      if (openTagPickerSourceId === targetSource.id) setOpenTagPickerSourceId(null)
      await Promise.all([loadFeed(), refreshStats()])
      setApiError('')
      showNotice('ニュースソースを削除しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const dropSource = async (targetSourceId: string) => {
    if (!draggedSourceId || draggedSourceId === targetSourceId || reorderingSourceId) {
      setDraggedSourceId(null)
      setDragOverSourceId(null)
      return
    }
    const previousOrder = managedSources
    const currentIndex = previousOrder.findIndex((item) => item.id === draggedSourceId)
    const targetIndex = previousOrder.findIndex((item) => item.id === targetSourceId)
    if (currentIndex < 0 || targetIndex < 0) return
    const nextOrder = [...previousOrder]
    const [moved] = nextOrder.splice(currentIndex, 1)
    nextOrder.splice(targetIndex, 0, moved)
    setManagedSources(nextOrder)
    setReorderingSourceId(draggedSourceId)
    try {
      const reordered = await api.reorderSources(nextOrder.map((item) => item.id))
      setManagedSources(reordered)
      setApiError('')
      showNotice('ニュースソースの優先順位を変更しました')
    } catch (error) {
      setManagedSources(previousOrder)
      showApiError(error)
    } finally {
      setReorderingSourceId(null)
      setDraggedSourceId(null)
      setDragOverSourceId(null)
    }
  }

  const refreshAllSources = async () => {
    if (refreshingSources) return
    setRefreshingSources(true)
    try {
      const result = await api.refreshAllSources()
      await Promise.all([loadData(), loadFeed()])
      setApiError('')
      if (result.failed) {
        showNotice(`${result.refreshed}件を更新し, ${result.failed}件失敗しました`)
      } else {
        showNotice(`${result.refreshed}件のニュースソースを更新しました`)
      }
    } catch (error) {
      showApiError(error)
    } finally {
      setRefreshingSources(false)
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
      setManagedTags((current) => current.map((item) => item.id === updated.id ? {
        ...updated,
        usageCount: item.usageCount,
        lastUsedAt: item.lastUsedAt,
      } : item))
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
      setLibraryArticles((current) => current.map((item) => ({
        ...item,
        tagIds: item.tagIds.filter((id) => id !== tagId),
      })))
      if (tag === tagId) setTag(ALL_TAGS)
      if (editingTagId === tagId) setEditingTagId(null)
      await refreshStats()
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
      setLibraryArticles((current) => current.map((item) => (
        item.sourceId === updated.id ? { ...item, tagIds: updated.tagIds } : item
      )))
      const tagChangeAffectsFeed = filterMode === 'tag' && (tag === UNTAGGED || tag === tagId)
      const [nextTags] = await Promise.all([
        api.listTags(),
        tagChangeAffectsFeed ? loadFeed() : Promise.resolve(),
        refreshStats(),
      ])
      setManagedTags(nextTags)
      setApiError('')
    } catch (error) {
      showApiError(error)
    }
  }

  const createAndAssignSourceTag = async (targetSource: Source, name: string) => {
    try {
      const created = await api.createTag(name)
      const updated = await api.setSourceTags(targetSource.id, [...targetSource.tagIds, created.id])
      setManagedSources((current) => current.map((item) => item.id === updated.id ? updated : item))
      setArticles((current) => current.map((item) => (
        item.sourceId === updated.id ? { ...item, tagIds: updated.tagIds } : item
      )))
      setLibraryArticles((current) => current.map((item) => (
        item.sourceId === updated.id ? { ...item, tagIds: updated.tagIds } : item
      )))
      const tagChangeAffectsFeed = filterMode === 'tag' && tag === UNTAGGED
      const [nextTags] = await Promise.all([
        api.listTags(),
        tagChangeAffectsFeed ? loadFeed() : Promise.resolve(),
        refreshStats(),
      ])
      setManagedTags(nextTags)
      setApiError('')
      showNotice('タグを作成して追加しました')
      return true
    } catch (error) {
      showApiError(error)
      return false
    }
  }

  const completeSwipe = useCallback(
    async (action: SwipeAction) => {
      if (!currentArticle || animating) return

      setAnimating(true)
      setDragging(false)
      try {
        await api.updateArticleState(currentArticle.id, action)
        const direction = action === 'save' ? 1 : -1
        setDragX(direction * Math.max(window.innerWidth * 0.7, 680))
        showNotice(action === 'save' ? 'あとで読むに保存しました' : '既読にしました')
        animationTimer.current = window.setTimeout(() => {
          setArticles((current) => current.filter((item) => item.id !== currentArticle.id))
          setProcessedCount((current) => current + 1)
          setDragX(0)
          setAnimating(false)
          void refreshStats().catch(showApiError)
        }, 260)
      } catch (error) {
        setDragX(0)
        setAnimating(false)
        showApiError(error)
      }
    },
    [animating, currentArticle, refreshStats, showApiError, showNotice],
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

  const markArticleRead = useCallback(async (article: Article, removeFromFeed = false) => {
    if (article.state.read) return
    try {
      await updateArticleState(article.id, 'read')
      if (removeFromFeed) {
        setArticles((current) => current.filter((item) => item.id !== article.id))
        setProcessedCount((current) => current + 1)
      }
      await refreshStats()
    } catch (error) {
      showApiError(error)
    }
  }, [refreshStats, showApiError, updateArticleState])

  const openOriginalArticle = useCallback((article: Article) => {
    if (!article.url) return
    window.open(article.url, '_blank', 'noopener,noreferrer')
    void markArticleRead(article, true)
  }, [markArticleRead])

  const toggleFavorite = async (articleId: string) => {
    const article = articles.find((item) => item.id === articleId)
      ?? libraryArticles.find((item) => item.id === articleId)
    if (!article) return
    try {
      await updateArticleState(articleId, article.state.favorite ? 'unfavorite' : 'favorite')
      await refreshStats()
      showNotice(article.state.favorite ? 'お気に入りから外しました' : 'お気に入りに追加しました')
    } catch (error) {
      showApiError(error)
    }
  }

  const deleteArticle = async (articleId: string) => {
    try {
      await updateArticleState(articleId, 'delete')
      setArticles((current) => current.filter((item) => item.id !== articleId))
      setProcessedCount((current) => current + 1)
      await refreshStats()
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
      setLibraryArticles((current) => current.filter((item) => item.id !== articleId))
      setLibraryTotal((current) => Math.max(0, current - 1))
      await refreshStats()
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
      await Promise.all([loadFeed(), refreshStats()])
      setDragX(0)
      setApiError('')
      showNotice(`${result.restored}件のスキップを解除しました`)
    } catch (error) {
      showApiError(error)
    }
  }

  const markAllArticlesRead = async () => {
    if (markingAllRead || articleStats.feed === 0) return
    if (!window.confirm(`${articleStats.feed}件の未読記事をすべて既読にしますか？`)) return
    setMarkingAllRead(true)
    try {
      const result = await api.markAllRead()
      setMenuOpen(false)
      setDragX(0)
      await Promise.all([loadFeed(), refreshStats()])
      setApiError('')
      showNotice(`${result.markedRead}件を既読にしました`)
    } catch (error) {
      showApiError(error)
    } finally {
      setMarkingAllRead(false)
    }
  }

  useEffect(() => {
    void loadData()
  }, [loadData])

  useEffect(() => {
    void loadFeed()
  }, [loadFeed])

  useEffect(() => {
    if (loading || prefetching || !nextCursor || articles.length > PREFETCH_THRESHOLD) return

    const requestID = feedRequestID.current
    const cursor = nextCursor
    setPrefetching(true)
    void api.listArticlePage(feedPageQuery(cursor)).then((page) => {
      if (requestID !== feedRequestID.current) return
      setArticles((current) => {
        const existing = new Set(current.map((item) => item.id))
        return [...current, ...page.items.filter((item) => !existing.has(item.id))]
      })
      setNextCursor(page.nextCursor)
      setApiError('')
    }).catch((error) => {
      if (requestID === feedRequestID.current) showApiError(error)
    }).finally(() => {
      if (requestID === feedRequestID.current) setPrefetching(false)
    })
  }, [articles.length, feedPageQuery, loading, nextCursor, prefetching, showApiError])

  useEffect(() => {
    setArticleExpanded(false)
  }, [currentArticle?.id])

  useEffect(() => {
    if (!libraryMode) {
      libraryRequestID.current += 1
      setLibraryArticles([])
      setLibraryNextCursor(undefined)
      setLibraryTotal(0)
      setLibraryLoading(false)
      return
    }

    const requestID = ++libraryRequestID.current
    setLibraryLoading(true)
    setLibraryArticles([])
    setLibraryNextCursor(undefined)
    void api.listArticlePage({ state: libraryMode, limit: LIBRARY_PAGE_SIZE }).then((page) => {
      if (requestID !== libraryRequestID.current) return
      setLibraryArticles(page.items)
      setLibraryNextCursor(page.nextCursor)
      setLibraryTotal(page.total)
      setApiError('')
    }).catch((error) => {
      if (requestID === libraryRequestID.current) showApiError(error)
    }).finally(() => {
      if (requestID === libraryRequestID.current) setLibraryLoading(false)
    })
  }, [libraryMode, showApiError])

  const loadMoreLibraryArticles = async () => {
    if (!libraryMode || !libraryNextCursor || libraryLoading) return
    const requestID = libraryRequestID.current
    setLibraryLoading(true)
    try {
      const page = await api.listArticlePage({
        state: libraryMode,
        cursor: libraryNextCursor,
        limit: LIBRARY_PAGE_SIZE,
      })
      if (requestID !== libraryRequestID.current) return
      setLibraryArticles((current) => [...current, ...page.items])
      setLibraryNextCursor(page.nextCursor)
      setApiError('')
    } catch (error) {
      if (requestID === libraryRequestID.current) showApiError(error)
    } finally {
      if (requestID === libraryRequestID.current) setLibraryLoading(false)
    }
  }

  useEffect(() => {
    const handleHashChange = () => {
      const nextLibraryMode = libraryModeFromHash()
      const nextTagManagerOpen = window.location.hash === '#/tags'
      const nextSourceManagerOpen = window.location.hash === '#/sources'
      setLibraryMode(nextLibraryMode)
      setTagManagerOpen(nextTagManagerOpen)
      setSourceManagerOpen(nextSourceManagerOpen)
      if (nextLibraryMode || nextTagManagerOpen || nextSourceManagerOpen) {
        setFocusMode(false)
        setArticleExpanded(false)
      }
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (articleExpanded) {
        if (event.key === 'Escape') setArticleExpanded(false)
        return
      }

      if (openTagPickerSourceId) {
        if (event.key === 'Escape') setOpenTagPickerSourceId(null)
        return
      }

      if (menuOpen) {
        if (event.key === 'Escape') setMenuOpen(false)
        return
      }

      if (libraryMode || sourceManagerOpen || tagManagerOpen) return

      if (event.key === 'Escape' && focusMode) setFocusMode(false)
      if (event.key === 'ArrowLeft') completeSwipe('skip')
      if (event.key === 'ArrowRight') completeSwipe('save')
      if (event.key === 'Enter' && currentArticle) openOriginalArticle(currentArticle)
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [articleExpanded, completeSwipe, currentArticle, focusMode, libraryMode, menuOpen, openOriginalArticle, openTagPickerSourceId, sourceManagerOpen, tagManagerOpen])

  useEffect(
    () => () => {
      if (animationTimer.current) window.clearTimeout(animationTimer.current)
      if (noticeTimer.current) window.clearTimeout(noticeTimer.current)
    },
    [],
  )

  const handlePointerDown: React.PointerEventHandler<HTMLElement> = (event) => {
    if (animating || articleExpanded) return
    activePointer.current = event.pointerId
    pointerStart.current = { x: event.clientX, y: event.clientY }
    pointerAxis.current = null
    setDragging(false)
  }

  const handlePointerMove: React.PointerEventHandler<HTMLElement> = (event) => {
    if (activePointer.current !== event.pointerId) return
    const deltaX = event.clientX - pointerStart.current.x
    const deltaY = event.clientY - pointerStart.current.y

    if (!pointerAxis.current) {
      if (Math.max(Math.abs(deltaX), Math.abs(deltaY)) < 8) return
      pointerAxis.current = Math.abs(deltaX) > Math.abs(deltaY) ? 'horizontal' : 'vertical'
      if (pointerAxis.current === 'horizontal') {
        setDragging(true)
        event.currentTarget.setPointerCapture(event.pointerId)
      }
    }

    if (pointerAxis.current === 'horizontal') setDragX(deltaX)
  }

  const handlePointerCancel: React.PointerEventHandler<HTMLElement> = (event) => {
    if (activePointer.current !== event.pointerId) return
    activePointer.current = null
    pointerAxis.current = null
    setDragging(false)
    setDragX(0)
  }

  const handlePointerUp: React.PointerEventHandler<HTMLElement> = (event) => {
    if (activePointer.current !== event.pointerId) return
    const finalDragX = event.clientX - pointerStart.current.x
    const completedHorizontalDrag = pointerAxis.current === 'horizontal'
    activePointer.current = null
    pointerAxis.current = null
    setDragging(false)

    if (!completedHorizontalDrag) {
      setDragX(0)
    } else if (finalDragX > SWIPE_THRESHOLD) {
      completeSwipe('save')
    } else if (finalDragX < -SWIPE_THRESHOLD) {
      completeSwipe('skip')
    } else {
      setDragX(0)
    }
  }

  const progress = feedTotal
    ? Math.min(100, (processedCount / feedTotal) * 100)
    : 0
  const remainingTagCount = (targetTag: TagFilter) => targetTag === ALL_TAGS
    ? articleStats.feed
    : targetTag === UNTAGGED
      ? articleStats.untaggedFeed
      : articleStats.tagFeedCounts[targetTag] ?? 0

  const remainingSourceCount = (targetSource: string) => targetSource === ALL_SOURCES
    ? articleStats.feed
    : articleStats.sourceFeedCounts[targetSource] ?? 0

  const activeFilterName = filterMode === 'source'
    ? sourceOptions.find((item) => item.id === source)?.name
    : tagOptions.find((item) => item.id === tag)?.name

  return (
    <div className={`app-shell${focusMode ? ' is-focus-mode' : ''}`}>
      <header className="topbar">
        <a className="brand" href="#/" aria-label="fliqrss ホーム">
          <span className="brand-mark"><span /></span>
          <span>fliq<span>rss</span></span>
        </a>

        <div className="topbar-status">
          <span className="live-dot" />
          <span>{loading ? 'API接続中' : apiError ? 'APIエラー' : 'API接続済み'}</span>
          <span className="status-divider" />
          <span>{feedTotal} stories</span>
        </div>

        <div className="header-menu">
          <button
            aria-label={markingAllRead ? 'すべての記事を既読にしています' : `すべて既読にする（未読${articleStats.feed}件）`}
            className="header-mark-all-read-button"
            disabled={markingAllRead || articleStats.feed === 0}
            onClick={() => void markAllArticlesRead()}
            title={markingAllRead ? '既読にしています' : 'すべて既読にする'}
            type="button"
          >
            <Icon name="check" size={20} />
          </button>
          <button
            aria-label={refreshingSources ? 'ニュースソースを更新中' : 'すべてのニュースソースを更新'}
            className={`header-refresh-button${refreshingSources ? ' is-refreshing' : ''}`}
            disabled={refreshingSources || !managedSources.some((item) => item.enabled)}
            onClick={() => void refreshAllSources()}
            title={refreshingSources ? '更新中' : 'ニュースソースを更新'}
            type="button"
          >
            <Icon name="refresh" size={20} />
          </button>
          <button
            aria-controls="main-menu"
            aria-expanded={menuOpen}
            aria-label={menuOpen ? 'メニューを閉じる' : 'メニューを開く'}
            className={`menu-toggle ${menuOpen ? 'is-open' : ''}`}
            onClick={() => setMenuOpen((current) => !current)}
            title={menuOpen ? 'メニューを閉じる' : 'メニューを開く'}
            type="button"
          >
            <Icon name={menuOpen ? 'close' : 'menu'} size={22} />
          </button>
          {menuOpen && (
            <>
              <button aria-label="メニューを閉じる" className="menu-scrim" onClick={() => setMenuOpen(false)} type="button" />
              <nav aria-label="メインメニュー" className="main-menu" id="main-menu">
                <a className={sourceManagerOpen ? 'is-active' : ''} href="#/sources" onClick={(event) => { event.preventDefault(); navigateToSources() }}>
                  <Icon name="rss" size={19} />
                  <span>ソース</span>
                  <strong>{managedSources.length}</strong>
                </a>
                <a className={tagManagerOpen ? 'is-active' : ''} href="#/tags" onClick={(event) => { event.preventDefault(); navigateToTags() }}>
                  <Icon name="tag" size={19} />
                  <span>タグ</span>
                  <strong>{managedTags.length}</strong>
                </a>
                <span className="main-menu-divider" />
                <a className={libraryMode === 'favorite' ? 'is-active' : ''} href="#/favorites" onClick={(event) => { event.preventDefault(); navigateToLibrary('favorite') }}>
                  <Icon name="star" size={19} />
                  <span>お気に入り</span>
                  <strong>{articleStats.favorite}</strong>
                </a>
                <a className={libraryMode === 'saved' ? 'is-active' : ''} href="#/saved" onClick={(event) => { event.preventDefault(); navigateToLibrary('saved') }}>
                  <Icon name="bookmark" size={19} />
                  <span>あとで見る</span>
                  <strong>{articleStats.saved}</strong>
                </a>
                <a className={libraryMode === 'deleted' ? 'is-active' : ''} href="#/deleted" onClick={(event) => { event.preventDefault(); navigateToLibrary('deleted') }}>
                  <Icon name="trash" size={19} />
                  <span>ゴミ箱</span>
                  <strong>{articleStats.deleted}</strong>
                </a>
                <span className="main-menu-divider" />
                <button
                  aria-label={`${theme === 'dark' ? 'ライト' : 'ダーク'}モードに切り替える`}
                  aria-pressed={theme === 'dark'}
                  className="theme-toggle"
                  onClick={toggleTheme}
                  title={`${theme === 'dark' ? 'ライト' : 'ダーク'}モードに切り替える`}
                  type="button"
                >
                  <Icon name={theme === 'dark' ? 'sun' : 'moon'} size={19} />
                  <span>{theme === 'dark' ? 'ライトモード' : 'ダークモード'}</span>
                  <strong>{theme === 'dark' ? 'ON' : 'OFF'}</strong>
                </button>
              </nav>
            </>
          )}
        </div>
      </header>

      {apiError && (
        <div className="api-error" role="alert">
          <span>{apiError}</span>
          <button aria-label="再試行" onClick={() => void Promise.all([loadData(), loadFeed()])} title="再試行" type="button"><Icon name="refresh" size={17} /></button>
        </div>
      )}

      {sourceManagerOpen ? (
        <main className="category-page" id="top">
          <div className="library-page-heading">
            <button aria-label="戻る" className="back-to-reader" onClick={navigateToReader} title="戻る" type="button">
              <Icon name="arrow-left" size={19} />
            </button>
            <p className="eyebrow">SOURCE SETTINGS</p>
            <h1>ニュースソース</h1>
            <p className="category-page-description">
              収集するRSS・Atomを追加し, 取得状態とタグの割り当てをニュースソースごとに管理できます. ドラッグして優先順位を変更し, ヘッダーの更新アイコンで重複判定へ反映します.
            </p>
          </div>

          <section className="opml-export-panel" aria-labelledby="opml-export-heading">
            <div>
              <p className="eyebrow">DATA EXPORT</p>
              <h2 id="opml-export-heading">データ出力</h2>
              <p>登録中のソースとタグを, OPMLファイルとして保存します.</p>
            </div>
            <button aria-label={exportingOPML ? 'OPMLを出力中' : 'OPMLを出力'} disabled={exportingOPML} onClick={() => void exportOPML()} title={exportingOPML ? '出力中' : 'OPML出力'} type="button">
              <Icon name="download" size={18} />
            </button>
          </section>

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
            <button aria-label={pending ? 'ニュースソースを取得中' : 'ニュースソースを追加'} className="source-add-button" disabled={pending || !newSourceName.trim() || !newSourceUrl.trim()} title={pending ? '取得中' : '追加'} type="submit">
              <Icon name="plus" size={18} />
            </button>
          </form>

          <section className="opml-import-panel" aria-labelledby="opml-import-heading">
            <div>
              <p className="eyebrow">OPML IMPORT</p>
              <h2 id="opml-import-heading">OPMLから一括追加</h2>
              <p>フォルダー階層はソースのタグとして取り込みます.</p>
            </div>
            <form onSubmit={importOPML}>
              <label className="opml-file-control" htmlFor="opml-file">
                <span>{opmlFile?.name ?? 'OPMLファイルを選択'}</span>
                <input
                  accept=".opml,.xml,application/xml,text/xml,text/x-opml"
                  id="opml-file"
                  onChange={(event) => {
                    setOPMLFile(event.target.files?.[0] ?? null)
                    setOPMLResult(null)
                  }}
                  ref={opmlInput}
                  type="file"
                />
              </label>
              <button aria-label={importingOPML ? 'OPMLを取込中' : 'OPMLを取り込む'} disabled={!opmlFile || importingOPML} title={importingOPML ? '取込中' : '取込'} type="submit">
                <Icon name="upload" size={18} />
              </button>
            </form>
            {opmlResult && (
              <dl className="opml-import-result" aria-label="OPML取込結果">
                <div><dt>追加</dt><dd>{opmlResult.added}件</dd></div>
                <div><dt>重複</dt><dd>{opmlResult.duplicates}件</dd></div>
                <div><dt>失敗</dt><dd>{opmlResult.failed}件</dd></div>
                <div><dt>新規タグ</dt><dd>{opmlResult.tagsCreated}件</dd></div>
              </dl>
            )}
          </section>

          <section className="source-manager-list" aria-label="ニュースソース一覧">
            {managedSources.map((item) => (
              <article
                className={`source-manager-item${dragOverSourceId === item.id ? ' is-drag-over' : ''}${draggedSourceId === item.id ? ' is-dragging' : ''}`}
                key={item.id}
                onDragOver={(event) => {
                  if (!draggedSourceId || reorderingSourceId) return
                  event.preventDefault()
                  event.dataTransfer.dropEffect = 'move'
                  setDragOverSourceId(item.id)
                }}
                onDrop={(event) => {
                  event.preventDefault()
                  void dropSource(item.id)
                }}
              >
                <div className="source-priority-controls">
                  <button
                    aria-label={`${item.name}をドラッグして並び替え`}
                    className="source-drag-handle"
                    disabled={Boolean(reorderingSourceId)}
                    draggable={!reorderingSourceId}
                    onDragEnd={() => {
                      setDraggedSourceId(null)
                      setDragOverSourceId(null)
                    }}
                    onDragStart={(event) => {
                      event.dataTransfer.effectAllowed = 'move'
                      event.dataTransfer.setData('text/plain', item.id)
                      setDraggedSourceId(item.id)
                    }}
                    title={`${item.name}を並び替え`}
                    type="button"
                  ><Icon name="drag-handle" size={27} /></button>
                </div>
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
                      title={`${item.name}のタグを設定`}
                      type="button"
                    >
                      <Icon name="plus" size={14} />
                    </button>
                    {openTagPickerSourceId === item.id && (
                      <>
                        <button aria-label="タグ一覧を閉じる" className="tag-picker-scrim" onClick={() => setOpenTagPickerSourceId(null)} type="button" />
                        <div className="tag-picker" role="dialog" aria-label={`${item.name}へ割り当てるタグ`}>
                          <div className="tag-picker-heading">
                            <strong>タグを選択</strong>
                            <button aria-label="閉じる" onClick={() => setOpenTagPickerSourceId(null)} title="閉じる" type="button"><Icon name="close" size={16} /></button>
                          </div>
                          <div className="tag-picker-list">
                            {managedTags.map((tagItem) => {
                              const selected = tagIdsForSource(item.id).includes(tagItem.id)
                              return (
                                <label
                                  className={selected ? 'is-active' : ''}
                                  key={tagItem.id}
                                >
                                  <input checked={selected} onChange={() => void toggleSourceTag(item, tagItem.id)} type="checkbox" />
                                  <span aria-hidden="true">{selected ? <Icon name="check" size={11} /> : null}</span>
                                  {tagItem.name}
                                </label>
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
                  <button
                    aria-label={`${item.name}の取得を${item.enabled ? '停止' : '再開'}`}
                    onClick={() => void toggleSourceEnabled(item)}
                    title={item.enabled ? '取得を停止' : '取得を再開'}
                    type="button"
                  >
                    <Icon name={item.enabled ? 'pause' : 'play'} size={16} />
                  </button>
                  <button
                    aria-label={`${item.name}を削除`}
                    className="category-delete-button"
                    onClick={() => deleteSource(item)}
                    title="ニュースソースを削除"
                    type="button"
                  >
                    <Icon name="trash" size={16} />
                  </button>
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
            <button aria-label="戻る" className="back-to-reader" onClick={navigateToReader} title="戻る" type="button">
              <Icon name="arrow-left" size={19} />
            </button>
            <p className="eyebrow">TAG SETTINGS</p>
            <h1>タグ一覧</h1>
            <p className="category-page-description">
              ニュースソースに設定可能なタグの追加, 名前変更, 削除ができます. タグの割り当てはソースページまたは記事カードで行います.
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
              <button aria-label="タグを追加" disabled={!newTagName.trim()} title="タグを追加" type="submit"><Icon name="plus" size={17} /></button>
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
                      <button aria-label="保存" disabled={!editingTagName.trim()} title="保存" type="submit"><Icon name="check" size={16} /></button>
                      <button aria-label="取消" onClick={() => setEditingTagId(null)} title="取消" type="button"><Icon name="close" size={16} /></button>
                    </form>
                  ) : (
                    <strong>{item.name}</strong>
                  )}
                  <span className="category-article-count">{sourceCount}件</span>
                  <div className="category-row-actions">
                    <button aria-label={`${item.name}を変更`} onClick={() => startEditingTag(item)} title="変更" type="button"><Icon name="edit" size={16} /></button>
                    <button aria-label={`${item.name}を削除`} className="category-delete-button" onClick={() => deleteTag(item.id)} title="削除" type="button"><Icon name="trash" size={16} /></button>
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
            <button aria-label="戻る" className="back-to-reader" onClick={navigateToReader} title="戻る" type="button">
              <Icon name="arrow-left" size={19} />
            </button>
            <p className="eyebrow">YOUR LIBRARY</p>
            <h1>記事ライブラリ</h1>
          </div>
          <div className="library-tabs" role="tablist" aria-label="記事の状態">
            <button aria-label={`お気に入り（${articleStats.favorite}件）`} className={libraryMode === 'favorite' ? 'is-active' : ''} onClick={() => navigateToLibrary('favorite')} title={`お気に入り（${articleStats.favorite}件）`} type="button">
              <Icon name="star" size={18} />
            </button>
            <button aria-label={`あとで見る（${articleStats.saved}件）`} className={libraryMode === 'saved' ? 'is-active' : ''} onClick={() => navigateToLibrary('saved')} title={`あとで見る（${articleStats.saved}件）`} type="button">
              <Icon name="bookmark" size={18} />
            </button>
            <button aria-label={`ゴミ箱（${articleStats.deleted}件）`} className={libraryMode === 'deleted' ? 'is-active' : ''} onClick={() => navigateToLibrary('deleted')} title={`ゴミ箱（${articleStats.deleted}件）`} type="button">
              <Icon name="trash" size={18} />
            </button>
          </div>
          <div className="library-list">
            {libraryArticles.map((article) => (
              <article className="library-item" key={article.id}>
                <div className={`queue-thumb queue-thumb--${article.visualTheme}`}>{article.sourceInitials}</div>
                {article.url ? (
                  <a
                    className="library-article-title"
                    href={article.url}
                    onClick={() => void markArticleRead(article, false)}
                    rel="noreferrer"
                    target="_blank"
                  >
                    <span>{tagNamesForSource(article.sourceId).join(' / ') || 'タグなし'} · {article.source}</span>
                    <strong>{article.title}</strong>
                  </a>
                ) : (
                  <div className="library-article-title is-disabled">
                    <span>{tagNamesForSource(article.sourceId).join(' / ') || 'タグなし'} · {article.source}</span>
                    <strong>{article.title}</strong>
                  </div>
                )}
                <button aria-label={libraryMode === 'deleted' ? '記事を復元' : '一覧から解除'} className="library-remove" onClick={() => removeFromLibrary(article.id)} title={libraryMode === 'deleted' ? '復元' : '解除'} type="button">
                  <Icon name={libraryMode === 'deleted' ? 'refresh' : 'close'} size={16} />
                </button>
              </article>
            ))}
            {libraryLoading && !libraryArticles.length && <div className="library-empty">記事を取得しています.</div>}
            {!libraryLoading && !libraryArticles.length && <div className="library-empty">該当する記事はありません.</div>}
            {libraryNextCursor && (
              <button
                aria-label={libraryLoading ? '記事を取得中' : `さらに表示（${libraryArticles.length} / ${libraryTotal}）`}
                className="library-load-more"
                disabled={libraryLoading}
                onClick={() => void loadMoreLibraryArticles()}
                title={libraryLoading ? '取得中' : 'さらに表示'}
                type="button"
              >
                <Icon name="plus" size={18} />
              </button>
            )}
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
              aria-label="ニュースソースで絞り込む"
              aria-selected={filterMode === 'source'}
              className={filterMode === 'source' ? 'is-active' : ''}
              onClick={() => selectFilterMode('source')}
              role="tab"
              title="ニュースソース"
              type="button"
            >
              <Icon name="rss" size={17} />
            </button>
            <button
              aria-label="タグで絞り込む"
              aria-selected={filterMode === 'tag'}
              className={filterMode === 'tag' ? 'is-active' : ''}
              onClick={() => selectFilterMode('tag')}
              role="tab"
              title="タグ"
              type="button"
            >
              <Icon name="tag" size={17} />
            </button>
          </div>

          <nav aria-label={filterMode === 'source' ? 'ニュースソース' : 'タグ'} className="category-nav filter-option-list">
            {(filterMode === 'source' ? sourceOptions : tagOptions).map((item) => (
              <a
                aria-current={(filterMode === 'source' ? source : tag) === item.id ? 'page' : undefined}
                className={(filterMode === 'source' ? source : tag) === item.id ? 'is-active' : ''}
                href="#top"
                key={item.id}
                onClick={(event) => {
                  event.preventDefault()
                  filterMode === 'source' ? selectSource(item.id) : selectTag(item.id)
                }}
              >
                <span>{item.name}</span>
                <span>
                  {filterMode === 'source' ? remainingSourceCount(item.id) : remainingTagCount(item.id)}
                </span>
              </a>
            ))}
          </nav>

          <div className="keyboard-help">
            <span><kbd>←</kbd> スキップ</span>
            <span><kbd>→</kbd> 保存</span>
          </div>
        </aside>

        <section className="reader-panel" aria-live="polite">
          <div className="mobile-filter-mode" aria-label="絞り込み方法">
            <button aria-label="ニュースソースで絞り込む" className={filterMode === 'source' ? 'is-active' : ''} onClick={() => selectFilterMode('source')} title="ニュースソース" type="button">
              <Icon name="rss" size={17} />
            </button>
            <button aria-label="タグで絞り込む" className={filterMode === 'tag' ? 'is-active' : ''} onClick={() => selectFilterMode('tag')} title="タグ" type="button">
              <Icon name="tag" size={17} />
            </button>
          </div>
          <div className="mobile-categories" aria-label={filterMode === 'source' ? 'ニュースソース' : 'タグ'}>
            {(filterMode === 'source' ? sourceOptions : tagOptions).map((item) => (
              <a
                className={(filterMode === 'source' ? source : tag) === item.id ? 'is-active' : ''}
                href="#top"
                key={item.id}
                onClick={(event) => {
                  event.preventDefault()
                  filterMode === 'source' ? selectSource(item.id) : selectTag(item.id)
                }}
              >
                {item.name}
              </a>
            ))}
          </div>

          <div className="reader-heading">
            <span>{filterMode === 'source' ? 'ニュースソース' : 'タグ'} · {activeFilterName ?? 'すべて'}</span>
            <div className="reader-heading-tools">
              <div className="progress-count">
                <strong>
                  {String(currentArticle ? processedCount + 1 : feedTotal).padStart(2, '0')}
                </strong>
                <span>/ {String(feedTotal).padStart(2, '0')}</span>
              </div>
              <button
                aria-label={focusMode ? '集中モードを終了' : '集中モードを開始'}
                aria-pressed={focusMode}
                className="focus-mode-toggle"
                onClick={() => setFocusMode((current) => !current)}
                title={focusMode ? '集中モードを終了' : '集中モード'}
                type="button"
              >
                <Icon name={focusMode ? 'collapse' : 'expand'} size={18} />
              </button>
            </div>
          </div>

          <div className="progress-track"><span style={{ width: `${progress}%` }} /></div>

          <div className={`card-stage ${animating ? 'is-advancing' : ''}`}>
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
                  key={currentArticle.id}
                  tags={managedTags}
                  tagIds={tagIdsForSource(currentArticle.sourceId)}
                  dragX={dragX}
                  dragging={dragging}
                  expanded={articleExpanded}
                  isFavorite={currentArticle.state.favorite}
                  onDelete={() => deleteArticle(currentArticle.id)}
                  onExpandedChange={setArticleExpanded}
                  onVisit={() => void markArticleRead(currentArticle, true)}
                  onToggleFavorite={() => toggleFavorite(currentArticle.id)}
                  onCreateTag={(name) => {
                    const targetSource = managedSources.find((item) => item.id === currentArticle.sourceId)
                    return targetSource ? createAndAssignSourceTag(targetSource, name) : Promise.resolve(false)
                  }}
                  onToggleTag={(tagId) => {
                    const targetSource = managedSources.find((item) => item.id === currentArticle.sourceId)
                    return targetSource ? toggleSourceTag(targetSource, tagId) : Promise.resolve()
                  }}
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
                {articleStats.skipped > 0 && (
                  <button aria-label="スキップを解除" onClick={restoreSkippedArticles} title="スキップ解除" type="button">
                    <Icon name="refresh" size={18} />
                  </button>
                )}
              </div>
            )}
          </div>

          <div className="reader-actions">
            <button
              aria-label="スキップ"
              className="action-button action-button--skip"
              disabled={!currentArticle || animating || articleExpanded}
              onClick={() => completeSwipe('skip')}
              title="スキップ"
              type="button"
            >
              <Icon name="close" size={22} />
            </button>
            <button
              aria-label="あとで見るに保存"
              className="action-button action-button--save"
              disabled={!currentArticle || animating || articleExpanded}
              onClick={() => completeSwipe('save')}
              title="保存"
              type="button"
            >
              <Icon name="bookmark" size={20} />
            </button>
          </div>
        </section>

        </main>
      )}

      {notice && <div className="notice" role="status"><Icon name="check" size={17} />{notice}</div>}

    </div>
  )
}

export default App
