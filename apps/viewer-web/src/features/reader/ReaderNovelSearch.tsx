import { useState, type FormEvent, type ReactNode } from "react";
import { searchNovelText } from "./api";
import type { NovelSearchMatch } from "./types";

type ReaderNovelSearchProps = {
  novelId: string;
  children: ReactNode;
  formatEpisodeLabel: (episodeIndex: string) => string;
  onRead: (match: NovelSearchMatch) => void;
};

export function ReaderNovelSearch({ novelId, children, formatEpisodeLabel, onRead }: ReaderNovelSearchProps) {
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<NovelSearchMatch[] | null>(null);
  const [preview, setPreview] = useState<NovelSearchMatch | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  async function handleSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalizedQuery = query.trim();
    if (!normalizedQuery || isLoading) {
      return;
    }
    setIsLoading(true);
    setError(null);
    setPreview(null);
    try {
      const result = await searchNovelText(novelId, normalizedQuery);
      setMatches(result.matches);
    } catch (searchError) {
      setMatches(null);
      setError(searchError instanceof Error ? searchError.message : "作品内検索に失敗しました。");
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <>
      <form className="reader-search-form" onSubmit={handleSearch}>
        <label className="reader-search-label" htmlFor="reader-search-query">
          作品内検索
        </label>
        <div className="reader-search-row">
          <input
            id="reader-search-query"
            maxLength={120}
            onInput={(event) => setQuery(event.currentTarget.value)}
            placeholder="人物名・用語・本文を検索"
            type="search"
            value={query}
          />
          <button disabled={!query.trim() || isLoading} type="submit">
            {isLoading ? "検索中…" : "検索"}
          </button>
        </div>
      </form>
      {error ? <p className="message error">{error}</p> : null}
      {preview ? (
        <section className="reader-panel-card reader-panel-card--hero reader-search-preview">
          <p className="reader-panel-chip">プレビュー</p>
          <h3>{preview.title}</h3>
          <p className="reader-search-snippet">{preview.snippet}</p>
          <p className="reader-search-preview-note">プレビューでは最終既読位置を変更しません。</p>
          <div className="reader-search-actions">
            <button className="secondary" onClick={() => setPreview(null)} type="button">
              検索結果に戻る
            </button>
            <button onClick={() => onRead(preview)} type="button">
              この位置から読む
            </button>
          </div>
        </section>
      ) : matches ? (
        <section className="reader-search-results" aria-label="作品内検索結果">
          <div className="reader-search-results-heading">
            <strong>{matches.length}件の検索結果</strong>
            <button
              className="reader-panel-link"
              onClick={() => {
                setMatches(null);
                setPreview(null);
              }}
              type="button"
            >
              目次に戻る
            </button>
          </div>
          {matches.length === 0 ? (
            <p className="message">一致する本文がありません。</p>
          ) : (
            <div className="reader-panel-card reader-panel-card--compact reader-search-result-list">
              {matches.map((match) => (
                <button
                  className="reader-panel-list-item reader-search-result"
                  key={`${match.episodeIndex}-${match.position}`}
                  onClick={() => setPreview(match)}
                  type="button"
                >
                  <span className="reader-panel-list-item-meta">{formatEpisodeLabel(match.episodeIndex)}</span>
                  <strong className="reader-panel-list-item-title">{match.title}</strong>
                  <span className="reader-search-snippet">{match.snippet}</span>
                </button>
              ))}
            </div>
          )}
        </section>
      ) : (
        children
      )}
    </>
  );
}
