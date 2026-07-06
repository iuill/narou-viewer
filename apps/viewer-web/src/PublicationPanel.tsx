import { useState, type FormEvent } from "react";
import { normalizeISBN13 } from "./features/publications/isbn";
import type { PublicationEntry, PublicationKind } from "./features/publications/types";
import { formatDate } from "./shared/date";

type Props = {
  displayCoverEntryId: string;
  entries: PublicationEntry[];
  isLoading: boolean;
  savingEntryId: string | null;
  onCreateISBN: (kind: PublicationKind, isbn13: string) => void | Promise<void>;
  onSaveISBN: (entryId: string, isbn13: string) => void | Promise<void>;
  onClear: (entry: PublicationEntry) => void | Promise<void>;
  onDisable: (entry: PublicationEntry) => void | Promise<void>;
  onRedisplay: (entry: PublicationEntry) => void | Promise<void>;
  onSetDisplayCover: (entryId: string) => void | Promise<void>;
};

const publicationKinds: Array<{ kind: PublicationKind; label: string; addLabel: string }> = [
  { kind: "novel", label: "小説版", addLabel: "小説版を追加" },
  { kind: "comic", label: "コミック版", addLabel: "コミック版を追加" }
];

function kindLabel(kind: PublicationKind): string {
  return kind === "novel" ? "小説版" : "コミック版";
}

function formatAuthors(entry: PublicationEntry): string {
  return entry.authors && entry.authors.length > 0 ? entry.authors.join("、") : "著者未取得";
}

function formatPublicationMeta(entry: PublicationEntry): string {
  const parts = [entry.publisher, entry.publishedDate].filter((value): value is string => Boolean(value));
  return parts.length > 0 ? parts.join(" / ") : "書誌情報未取得";
}

function hasProvider(entries: PublicationEntry[], provider: string): boolean {
  return entries.some((entry) => {
    if (entry.status === "disabled") {
      return false;
    }
    const providers = [entry.source, entry.coverSource, entry.providerId].filter(Boolean).join(" ");
    return providers.includes(provider);
  });
}

function formatSourceLinkLabel(source: string | undefined, fallback: string): string {
  return source ? `${source}で見る` : fallback;
}

function isGoogleBooksEntry(entry: PublicationEntry): boolean {
  return entry.coverSource === "Google Books";
}

function formatWarningMessages(entry: PublicationEntry): string[] {
  const warnings = new Set(entry.warnings ?? []);
  const messages: string[] = [];
  if (warnings.has("ndl_lookup_failed")) {
    messages.push("NDLサーチから書誌情報を取得できませんでした。");
  }
  if (warnings.has("google_books_lookup_failed")) {
    messages.push("Google Books から表紙画像を取得できませんでした。");
  }
  if (warnings.has("google_books_api_key_missing")) {
    messages.push("Google Books API key が未設定のため、表紙画像を取得しませんでした。");
  }
  if (warnings.has("google_books_cover_missing")) {
    messages.push("Google Books に書誌はありますが、表紙画像は提供されていません。");
  }
  const knownWarningCount = messages.length;
  if ((entry.warnings?.length ?? 0) > knownWarningCount) {
    messages.push("一部の書籍情報を取得できませんでした。");
  }
  return messages;
}

function validateISBNDraft(rawISBN: string): string {
  return rawISBN.trim().length > 0 ? normalizeISBN13(rawISBN) : "";
}

function resolveFallbackDisplayCoverEntryId(entries: PublicationEntry[]): string {
  for (const kind of ["novel", "comic"] as const) {
    const entry = entries.find((candidate) => candidate.kind === kind && candidate.status !== "disabled" && Boolean(candidate.imageUrl));
    if (entry) {
      return entry.id;
    }
  }
  return "";
}

export function PublicationPanel({
  displayCoverEntryId,
  entries,
  isLoading,
  savingEntryId,
  onCreateISBN,
  onSaveISBN,
  onClear,
  onDisable,
  onRedisplay,
  onSetDisplayCover
}: Props) {
  const [isbnDrafts, setIsbnDrafts] = useState<Record<string, string>>({});
  const [isbnErrors, setIsbnErrors] = useState<Record<string, string>>({});
  const usesNDL = hasProvider(entries, "NDL") || hasProvider(entries, "ndl");
  const usesGoogleBooks = hasProvider(entries, "Google Books") || hasProvider(entries, "google_books");
  const fallbackDisplayCoverEntryId = displayCoverEntryId === "" ? resolveFallbackDisplayCoverEntryId(entries) : "";

  function handleEntrySubmit(entry: PublicationEntry, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const rawISBN = (isbnDrafts[entry.id] ?? "").trim();
    const isbn13 = validateISBNDraft(rawISBN);
    if (isbn13 === "") {
      setIsbnErrors((current) => ({ ...current, [entry.id]: "ISBN13 が正しくありません。" }));
      return;
    }
    setIsbnErrors((current) => ({ ...current, [entry.id]: "" }));
    void onSaveISBN(entry.id, isbn13);
  }

  function handleCreateSubmit(kind: PublicationKind, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const draftKey = `create-${kind}`;
    const rawISBN = (isbnDrafts[draftKey] ?? "").trim();
    const isbn13 = validateISBNDraft(rawISBN);
    if (isbn13 === "") {
      setIsbnErrors((current) => ({ ...current, [draftKey]: "ISBN13 が正しくありません。" }));
      return;
    }
    setIsbnErrors((current) => ({ ...current, [draftKey]: "" }));
    void onCreateISBN(kind, isbn13);
  }

  function handleISBNDraftChange(key: string, value: string) {
    setIsbnDrafts((current) => ({ ...current, [key]: value }));
    if (isbnErrors[key] !== "") {
      setIsbnErrors((current) => ({ ...current, [key]: "" }));
    }
  }

  return (
    <section className="publication-block">
      <div className="panel-header compact">
        <h3>書籍情報</h3>
        <p>{isLoading ? "確認中" : "ISBN 登録で表紙を取得"}</p>
      </div>
      <div className="publication-list">
        {publicationKinds.map(({ kind, label, addLabel }) => {
          const kindEntries = entries.filter((entry) => entry.kind === kind);
          const createKey = `create-${kind}`;
          const createDraft = isbnDrafts[createKey] ?? "";
          const createISBN = validateISBNDraft(createDraft);
          const isCreateInvalid = createDraft.trim().length > 0 && createISBN === "";
          return (
            <section className="publication-kind-group" key={kind}>
              <div className="publication-kind-heading">
                <strong>{label}</strong>
                <span>{kindEntries.filter((entry) => entry.status !== "unknown" && entry.status !== "disabled").length} 件</span>
              </div>
              {kindEntries.map((entry, index) => {
                const isSaving = savingEntryId === entry.id;
                const draft = isbnDrafts[entry.id] ?? "";
                const normalizedDraft = validateISBNDraft(draft);
                const isInvalidDraft = draft.trim().length > 0 && normalizedDraft === "";
                const warningMessages = formatWarningMessages(entry);
                const title = entry.title || entry.isbn13 || "未登録";
                const isDisabled = entry.status === "disabled";
                const visibleEntry = entry.status !== "unknown" && !isDisabled ? entry : null;
                const shouldShowCover = Boolean(visibleEntry?.imageUrl);
                const isDisplayCover = displayCoverEntryId !== "" && displayCoverEntryId === entry.id;
                const isFallbackDisplayCover = fallbackDisplayCoverEntryId === entry.id;
                const labelWithIndex = kindEntries.length > 1 ? `${kindLabel(kind)} ${index + 1}` : kindLabel(kind);
                return (
                  <article className={`publication-card ${shouldShowCover ? "has-cover" : ""}`} key={entry.id}>
                    <div className="publication-cover" aria-hidden="true">
                      {shouldShowCover && entry.imageUrl ? <img alt="" src={entry.imageUrl} /> : <span>{labelWithIndex}</span>}
                    </div>
                    <div className="publication-body">
                      <div className="publication-heading">
                        <span>{labelWithIndex}</span>
                        <strong>{isDisabled ? "表示しない" : title}</strong>
                      </div>
                      {visibleEntry ? (
                        <dl className="publication-meta">
                          <div>
                            <dt>ISBN</dt>
                            <dd>{visibleEntry.isbn13 ?? "未取得"}</dd>
                          </div>
                          <div>
                            <dt>著者</dt>
                            <dd>{formatAuthors(visibleEntry)}</dd>
                          </div>
                          <div>
                            <dt>出版</dt>
                            <dd>{formatPublicationMeta(visibleEntry)}</dd>
                          </div>
                          <div>
                            <dt>更新</dt>
                            <dd>{visibleEntry.updatedAt ? formatDate(visibleEntry.updatedAt) : "未取得"}</dd>
                          </div>
                        </dl>
                      ) : (
                        <p className="publication-empty">
                          {isDisabled ? "この書籍情報は非表示に設定されています。" : "ISBN13 を登録すると表紙と書誌を補完します。"}
                        </p>
                      )}
                      {visibleEntry?.sourceUrl ? (
                        <a className="publication-source-link" href={visibleEntry.sourceUrl} rel="noreferrer" target="_blank">
                          {formatSourceLinkLabel(visibleEntry.source, "外部サイトで見る")}
                        </a>
                      ) : null}
                      {visibleEntry?.coverSourceUrl &&
                      visibleEntry.coverSourceUrl !== visibleEntry.sourceUrl &&
                      !isGoogleBooksEntry(visibleEntry) ? (
                        <a className="publication-source-link" href={visibleEntry.coverSourceUrl} rel="noreferrer" target="_blank">
                          {formatSourceLinkLabel(visibleEntry.coverSource, "カバー提供元で見る")}
                        </a>
                      ) : null}
                      {visibleEntry && isGoogleBooksEntry(visibleEntry) ? (
                        <p className="publication-provider-credit">
                          {visibleEntry.imageUrl ? "Cover data powered by Google." : "Book data powered by Google."}
                          {visibleEntry.coverSourceUrl ? (
                            <>
                              {" "}
                              <a href={visibleEntry.coverSourceUrl} rel="noreferrer" target="_blank">
                                Google Books で見る
                              </a>
                            </>
                          ) : null}
                        </p>
                      ) : null}
                      {visibleEntry && warningMessages.length > 0 ? (
                        <p className="publication-warning">ISBN は保存しました。{warningMessages.join(" ")}</p>
                      ) : null}
                      {visibleEntry?.imageUrl ? (
                        <button
                          className="summary-link-button publication-display-cover-button"
                          disabled={isSaving || isDisplayCover}
                          onClick={() => void onSetDisplayCover(entry.id)}
                          type="button"
                        >
                          {isDisplayCover
                            ? "一覧表紙に設定中"
                            : isFallbackDisplayCover
                              ? "自動表示中・固定する"
                              : "一覧表紙にする"}
                        </button>
                      ) : null}
                      {isDisabled ? (
                        <div className="publication-form publication-form--actions">
                          <button disabled={isSaving} onClick={() => void onRedisplay(entry)} type="button">
                            再表示
                          </button>
                          {entry.isbn13 ? (
                            <button disabled={isSaving} onClick={() => void onClear(entry)} type="button">
                              解除
                            </button>
                          ) : null}
                        </div>
                      ) : (
                        <>
                          <form className="publication-form" onSubmit={(event) => handleEntrySubmit(entry, event)}>
                            <input
                              aria-label={`${labelWithIndex} ISBN13`}
                              disabled={isSaving}
                              inputMode="numeric"
                              onChange={(event) => handleISBNDraftChange(entry.id, event.target.value)}
                              onInput={(event) => handleISBNDraftChange(entry.id, event.currentTarget.value)}
                              placeholder="ISBN13"
                              type="text"
                              value={draft}
                            />
                            <button disabled={isSaving || draft.trim().length === 0 || isInvalidDraft} type="submit">
                              {isSaving ? "保存中..." : entry.status === "unknown" ? "登録" : "更新"}
                            </button>
                            <button disabled={isSaving} onClick={() => void onClear(entry)} type="button">
                              解除
                            </button>
                            <button disabled={isSaving} onClick={() => void onDisable(entry)} type="button">
                              非表示
                            </button>
                          </form>
                          {isbnErrors[entry.id] ? <p className="publication-form-error">{isbnErrors[entry.id]}</p> : null}
                        </>
                      )}
                    </div>
                  </article>
                );
              })}
              <form className="publication-form publication-form--add" onSubmit={(event) => handleCreateSubmit(kind, event)}>
                <input
                  aria-label={`${addLabel} ISBN13`}
                  disabled={savingEntryId === createKey}
                  inputMode="numeric"
                  onChange={(event) => handleISBNDraftChange(createKey, event.target.value)}
                  onInput={(event) => handleISBNDraftChange(createKey, event.currentTarget.value)}
                  placeholder="ISBN13"
                  type="text"
                  value={createDraft}
                />
                <button disabled={savingEntryId === createKey || createDraft.trim().length === 0 || isCreateInvalid} type="submit">
                  {savingEntryId === createKey ? "追加中..." : addLabel}
                </button>
              </form>
              {isbnErrors[createKey] ? <p className="publication-form-error">{isbnErrors[createKey]}</p> : null}
            </section>
          );
        })}
      </div>
      {usesNDL || usesGoogleBooks ? (
        <p className="publication-provider-note">
          {usesNDL ? "書籍メタデータの一部は NDLサーチ API から取得しています。" : null}
          {usesNDL && usesGoogleBooks ? " " : null}
          {usesGoogleBooks ? "Google Books 由来の表紙・書誌情報を表示する場合があります。" : null}
        </p>
      ) : null}
    </section>
  );
}
