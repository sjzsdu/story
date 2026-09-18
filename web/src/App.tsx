import { Routes, Route, Link, useLocation } from 'react-router-dom'
import SeriesListPage from './pages/SeriesListPage'
import SeriesDetailPage from './pages/SeriesDetailPage'
import EpisodePage from './pages/EpisodePage'

function Brand() {
  return (
    <Link to="/" className="flex items-center gap-3 group">
      <span className="grid place-items-center w-9 h-9 rounded-md bg-seal-600 font-display text-xl leading-none shadow-[0_2px_10px_rgba(179,64,45,0.45)]">
        史
      </span>
      <span className="font-display text-xl tracking-widest text-paper-100 group-hover:text-gold-500 transition-colors">
        story
      </span>
      <span className="hidden sm:inline text-xs text-paper-300/50 border-l border-ink-600 pl-3">
        历史故事视频工坊
      </span>
    </Link>
  )
}

export default function App() {
  const loc = useLocation()
  return (
    <div className="min-h-full flex flex-col">
      <header className="sticky top-0 z-20 border-b border-ink-800 bg-ink-950/85 backdrop-blur">
        <div className="max-w-6xl mx-auto px-5 h-14 flex items-center justify-between">
          <Brand />
          <nav className="text-sm text-paper-300/70">
            <Link
              to="/"
              className={`hover:text-paper-100 ${loc.pathname === '/' ? 'text-gold-500' : ''}`}
            >
              系列
            </Link>
          </nav>
        </div>
      </header>

      <main className="flex-1 w-full max-w-6xl mx-auto px-5 py-8">
        <Routes>
          <Route path="/" element={<SeriesListPage />} />
          <Route path="/series/:seriesId" element={<SeriesDetailPage />} />
          <Route path="/series/:seriesId/episodes/:episodeId" element={<EpisodePage />} />
        </Routes>
      </main>

      <footer className="border-t border-ink-800 py-5 text-center text-xs text-paper-300/35">
        story · 典籍取材 · AI 分镜生产 · 断点续跑
      </footer>
    </div>
  )
}
