import { countGraphemes } from "./readerPosition";
import type {
  ReaderBlock,
  ReaderDocumentResponse,
  ReaderInlineToken,
  ReaderSectionRole
} from "./features/reader/types";

export type { ReaderBlock, ReaderInlineToken, ReaderSectionRole } from "./features/reader/types";
export type ReaderDocument = ReaderDocumentResponse;

type PaginationFragmentKind = "text" | "ruby" | "tcy" | "meta" | "title" | "image" | "html";

type PositionCursor = {
  value: number;
};

type RenderReaderDocumentOptions = {
  enableVisibilityFragments?: boolean;
};

const SAFE_URL_SCHEMES = new Set(["http", "https", "mailto"]);
const READER_DASH_RUN_MAX_INLINE_LENGTH = 12;
const graphemeSegmenter =
  typeof Intl !== "undefined" && "Segmenter" in Intl ? new Intl.Segmenter("ja", { granularity: "grapheme" }) : null;

function escapeHtml(source: string): string {
  return source
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttribute(source: string): string {
  return escapeHtml(source);
}

function hasControlCharacter(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code <= 0x1f || code === 0x7f) {
      return true;
    }
  }
  return false;
}

function sanitizeUrl(value: string | null | undefined): string | null {
  if (typeof value !== "string") {
    return null;
  }

  const trimmed = value.trim();
  if (trimmed.length === 0 || hasControlCharacter(trimmed)) {
    return null;
  }

  if (
    trimmed.startsWith("//") ||
    trimmed.startsWith("/") ||
    trimmed.startsWith("./") ||
    trimmed.startsWith("../") ||
    trimmed.startsWith("#") ||
    trimmed.startsWith("?")
  ) {
    return trimmed;
  }

  const schemeMatch = trimmed.match(/^([a-zA-Z][a-zA-Z\d+.-]*):/);
  if (!schemeMatch) {
    return trimmed;
  }

  return SAFE_URL_SCHEMES.has(schemeMatch[1]?.toLowerCase()) ? trimmed : null;
}

function renderPositionAttributes(start: number, end: number): string {
  if (end <= start) {
    return "";
  }

  return ` data-reader-position-start="${String(start)}" data-reader-position-end="${String(end)}"`;
}

function advancePosition(cursor: PositionCursor, length: number): { start: number; end: number } | null {
  if (length <= 0) {
    return null;
  }

  const start = cursor.value;
  cursor.value += length;
  return {
    start,
    end: cursor.value
  };
}

function renderPaginationFragment(
  kind: PaginationFragmentKind,
  content: string,
  className?: string,
  range?: { start: number; end: number } | null
): string {
  const classAttribute = typeof className === "string" ? ` class="${escapeAttribute(className)}"` : "";
  const positionAttributes = range ? renderPositionAttributes(range.start, range.end) : "";
  return `<span data-reader-pagination-fragment="${kind}"${classAttribute}${positionAttributes}>${content}</span>`;
}

function splitDashRunLength(runLength: number): number[] {
  const chunks: number[] = [];
  for (let remaining = runLength; remaining > 0; remaining -= READER_DASH_RUN_MAX_INLINE_LENGTH) {
    chunks.push(Math.min(remaining, READER_DASH_RUN_MAX_INLINE_LENGTH));
  }
  return chunks;
}

function dashRunClassName(chunkIndex: number, chunkCount: number): string {
  if (chunkCount === 1) {
    return "reader-dash-run";
  }
  const edgeClass =
    chunkIndex === 0
      ? "reader-dash-run-split-start"
      : chunkIndex === chunkCount - 1
        ? "reader-dash-run-split-end"
        : "reader-dash-run-split-middle";
  return `reader-dash-run ${edgeClass}`;
}

function renderDashRun(content: string, className = "reader-dash-run"): string {
  return `<span class="${className}">${content}</span>`;
}

const HTML_INLINE_DASH_RUN_TAGS = new Set([
  "a",
  "abbr",
  "b",
  "bdi",
  "bdo",
  "cite",
  "code",
  "data",
  "em",
  "i",
  "kbd",
  "mark",
  "q",
  "rb",
  "rp",
  "rt",
  "rtc",
  "ruby",
  "s",
  "samp",
  "small",
  "span",
  "strong",
  "sub",
  "sup",
  "time",
  "u",
  "var",
  "wbr"
]);

type HtmlDashTextRef = {
  node: Text;
  index: number;
};

type HtmlDashRunSpan = {
  start: number;
  end: number;
  className: string;
};

function connectedDashRunPartClassName(partIndex: number, partCount: number): string {
  if (partCount === 1) {
    return "reader-dash-run";
  }
  const edgeClass =
    partIndex === 0
      ? "reader-dash-run-split-start"
      : partIndex === partCount - 1
        ? "reader-dash-run-split-end"
        : "reader-dash-run-split-middle";
  return `reader-dash-run ${edgeClass}`;
}

function collectHtmlDashRunSpans(root: DocumentFragment): Map<Text, HtmlDashRunSpan[]> {
  const spansByTextNode = new Map<Text, HtmlDashRunSpan[]>();
  let pending: HtmlDashTextRef[] = [];

  const addSpan = (node: Text, span: HtmlDashRunSpan) => {
    const spans = spansByTextNode.get(node) ?? [];
    spans.push(span);
    spansByTextNode.set(node, spans);
  };

  const flush = () => {
    if (pending.length >= 2) {
      const contiguousNodeRuns: Array<{ node: Text; start: number; end: number }> = [];
      for (const ref of pending) {
        const lastRun = contiguousNodeRuns.at(-1);
        if (lastRun && lastRun.node === ref.node && lastRun.end === ref.index) {
          lastRun.end = ref.index + 1;
        } else {
          contiguousNodeRuns.push({
            node: ref.node,
            start: ref.index,
            end: ref.index + 1
          });
        }
      }

      if (contiguousNodeRuns.length === 1) {
        const [run] = contiguousNodeRuns;
        if (run) {
          const chunkLengths = splitDashRunLength(run.end - run.start);
          let cursor = run.start;
          for (const [chunkIndex, chunkLength] of chunkLengths.entries()) {
            addSpan(run.node, {
              start: cursor,
              end: cursor + chunkLength,
              className: dashRunClassName(chunkIndex, chunkLengths.length)
            });
            cursor += chunkLength;
          }
        }
      } else {
        for (const [partIndex, run] of contiguousNodeRuns.entries()) {
          addSpan(run.node, {
            start: run.start,
            end: run.end,
            className: connectedDashRunPartClassName(partIndex, contiguousNodeRuns.length)
          });
        }
      }
    }
    pending = [];
  };

  const walk = (nodes: Iterable<ChildNode>) => {
    for (const node of nodes) {
      if (node.nodeType === 3) {
        const textNode = node as Text;
        splitIntoGraphemeSegments(textNode.data).forEach((segment, index) => {
          if (segment === "―") {
            pending.push({ node: textNode, index });
          } else {
            flush();
          }
        });
        continue;
      }

      if (node.nodeType !== 1) {
        flush();
        continue;
      }

      const element = node as Element;
      const tagName = element.tagName.toLowerCase();
      if (tagName === "br") {
        flush();
        continue;
      }
      if (!HTML_INLINE_DASH_RUN_TAGS.has(tagName)) {
        flush();
        walk(Array.from(element.childNodes));
        flush();
        continue;
      }
      walk(Array.from(element.childNodes));
    }
  };

  walk(Array.from(root.childNodes));
  flush();

  for (const spans of spansByTextNode.values()) {
    spans.sort((a, b) => a.start - b.start);
  }

  return spansByTextNode;
}

function appendHtmlDashRunFragmentsToDom(
  ownerDocument: Document,
  fragment: DocumentFragment,
  text: string,
  dashRunSpans: HtmlDashRunSpan[]
): void {
  const segments = splitIntoGraphemeSegments(text);
  let index = 0;
  let spanIndex = 0;

  while (index < segments.length) {
    const span = dashRunSpans[spanIndex];
    if (!span || index < span.start) {
      const nextIndex = span ? Math.min(span.start, segments.length) : segments.length;
      fragment.append(ownerDocument.createTextNode(segments.slice(index, nextIndex).join("")));
      index = nextIndex;
      continue;
    }

    if (index === span.start) {
      const dashRun = ownerDocument.createElement("span");
      dashRun.className = span.className;
      dashRun.textContent = segments.slice(span.start, span.end).join("");
      fragment.append(dashRun);
      index = span.end;
      spanIndex += 1;
      continue;
    }

    index += 1;
  }
}

function renderHtmlFallbackDashRuns(html: string): string {
  const ownerDocument = typeof document === "undefined" ? null : document;
  if (!ownerDocument) {
    return html;
  }

  const template = ownerDocument.createElement("template");
  template.innerHTML = html;
  const dashRunSpans = collectHtmlDashRunSpans(template.content);
  const showText = ownerDocument.defaultView?.NodeFilter?.SHOW_TEXT ?? 4;
  const walker = ownerDocument.createTreeWalker(template.content, showText);
  const textNodes: Text[] = [];
  while (walker.nextNode()) {
    if (walker.currentNode.nodeType === ownerDocument.TEXT_NODE) {
      textNodes.push(walker.currentNode as Text);
    }
  }

  for (const textNode of textNodes) {
    const spans = dashRunSpans.get(textNode);
    if (spans) {
      const fragment = ownerDocument.createDocumentFragment();
      appendHtmlDashRunFragmentsToDom(ownerDocument, fragment, textNode.data, spans);
      textNode.replaceWith(fragment);
    }
  }

  return template.innerHTML;
}

function splitIntoGraphemeSegments(text: string): string[] {
  if (text.length === 0) {
    return [];
  }

  if (graphemeSegmenter) {
    return Array.from(graphemeSegmenter.segment(text), (segment) => segment.segment);
  }

  return Array.from(text);
}

function renderDashRunFragments(kind: PaginationFragmentKind, runLength: number, enabled: boolean): string {
  const chunkLengths = splitDashRunLength(runLength);
  return chunkLengths
    .map((chunkLength, chunkIndex) => {
      const className = dashRunClassName(chunkIndex, chunkLengths.length);
      if (!enabled) {
        return renderDashRun("―".repeat(chunkLength), className);
      }

      return renderDashRun(
        Array.from({ length: chunkLength }, () => `<span data-reader-visibility-fragment="${kind}">―</span>`).join(""),
        className
      );
    })
    .join("");
}

function renderVisibilityFragments(kind: PaginationFragmentKind, text: string, enabled: boolean): string {
  const segments = splitIntoGraphemeSegments(text);
  if (segments.length === 0) {
    return "";
  }

  const htmlParts: string[] = [];
  for (let index = 0; index < segments.length;) {
    if (segments[index] === "―") {
      const start = index;
      while (index < segments.length && segments[index] === "―") {
        index += 1;
      }
      const runLength = index - start;
      if (runLength >= 2) {
        htmlParts.push(renderDashRunFragments(kind, runLength, enabled));
        continue;
      }
    }

    const segment = segments[index] ?? "";
    htmlParts.push(enabled ? `<span data-reader-visibility-fragment="${kind}">${escapeHtml(segment)}</span>` : escapeHtml(segment));
    index += 1;
  }

  return htmlParts.join("");
}

function renderInlineTokens(tokens: ReaderInlineToken[], cursor: PositionCursor, options: RenderReaderDocumentOptions): string {
  const enableVisibilityFragments = options.enableVisibilityFragments === true;

  return tokens
    .map((token) => {
      if (token.type === "text") {
        return renderPaginationFragment(
          "text",
          renderVisibilityFragments("text", token.text, enableVisibilityFragments),
          undefined,
          advancePosition(cursor, countGraphemes(token.text))
        );
      }

      if (token.type === "lineBreak") {
        const range = advancePosition(cursor, 1);
        const positionAttributes = range ? renderPositionAttributes(range.start, range.end) : "";
        return `<br${positionAttributes}>`;
      }

      if (token.type === "ruby") {
        const range = advancePosition(cursor, countGraphemes(token.text));
        const positionAttributes = range ? renderPositionAttributes(range.start, range.end) : "";
        return `<ruby data-reader-pagination-fragment="ruby"${positionAttributes}>${renderVisibilityFragments("ruby", token.text, enableVisibilityFragments)}<rt>${escapeHtml(token.ruby)}</rt></ruby>`;
      }

      if (token.type === "tcy") {
        return renderPaginationFragment(
          "tcy",
          renderVisibilityFragments("tcy", token.text, enableVisibilityFragments),
          "reader-tcy",
          advancePosition(cursor, countGraphemes(token.text))
        );
      }

      const hrefValue = sanitizeUrl(token.href);
      const href = typeof hrefValue === "string" ? ` href="${escapeAttribute(hrefValue)}"` : "";
      return `<a${href}>${renderInlineTokens(token.children, cursor, options)}</a>`;
    })
    .join("");
}

function renderBlockHtml(block: ReaderBlock, cursor: PositionCursor, options: RenderReaderDocumentOptions): string {
  const enableVisibilityFragments = options.enableVisibilityFragments === true;

  if (block.type === "meta") {
    return `<p class="reader-meta">${renderPaginationFragment("meta", renderVisibilityFragments("meta", block.text, enableVisibilityFragments), undefined, advancePosition(cursor, countGraphemes(block.text)))}</p>`;
  }

  if (block.type === "title") {
    return `<h1 class="reader-title">${renderPaginationFragment("title", renderVisibilityFragments("title", block.text, enableVisibilityFragments), undefined, advancePosition(cursor, countGraphemes(block.text)))}</h1>`;
  }

  if (block.type === "paragraph") {
    return `<p>${renderInlineTokens(block.inlines, cursor, options)}</p>`;
  }

  if (block.type === "image") {
    const src = sanitizeUrl(block.src);
    if (typeof src !== "string") {
      return "";
    }

    const alt = typeof block.alt === "string" ? ` alt="${escapeAttribute(block.alt)}"` : "";
    const title = typeof block.title === "string" ? ` title="${escapeAttribute(block.title)}"` : "";
    const width = typeof block.width === "number" && block.width > 0 ? ` width="${block.width}"` : "";
    const height = typeof block.height === "number" && block.height > 0 ? ` height="${block.height}"` : "";
    const originalUrl = sanitizeUrl(block.originalUrl);
    const originalHref =
      typeof originalUrl === "string"
        ? ` data-reader-image-original-href="${escapeAttribute(originalUrl)}"`
        : "";
    const originalTitle =
      typeof block.title === "string"
        ? ` data-reader-image-original-title="${escapeAttribute(block.title)}"`
        : "";
    const range = advancePosition(cursor, 1);
    const positionAttributes = range ? renderPositionAttributes(range.start, range.end) : "";
    return `<p><img src="${escapeAttribute(src)}"${alt}${title}${width}${height}${originalHref}${originalTitle} data-reader-pagination-fragment="image"${positionAttributes}></p>`;
  }

  const range = advancePosition(cursor, 1);
  const positionAttributes = range ? renderPositionAttributes(range.start, range.end) : "";
  return `<div class="reader-html-fragment" data-reader-pagination-fragment="html"${positionAttributes}>${renderHtmlFallbackDashRuns(block.html)}</div>`;
}

export function renderReaderDocumentHtml(readerDocument: ReaderDocument, options: RenderReaderDocumentOptions = {}): string {
  const htmlParts: string[] = [];
  let activeSection: ReaderSectionRole | null = null;
  let activeSectionParts: string[] = [];
  const cursor: PositionCursor = { value: 0 };

  const flushSection = () => {
    if (!activeSection || activeSectionParts.length === 0) {
      activeSection = null;
      activeSectionParts = [];
      return;
    }

    htmlParts.push(`<section class="reader-section reader-section-${activeSection}">${activeSectionParts.join("")}</section>`);
    activeSection = null;
    activeSectionParts = [];
  };

  for (const block of readerDocument.blocks) {
    const nextSection = "section" in block ? block.section : null;
    if (nextSection !== activeSection) {
      flushSection();
      activeSection = nextSection;
    }

    const html = renderBlockHtml(block, cursor, options);
    if (html.length === 0) {
      continue;
    }

    if (nextSection) {
      activeSectionParts.push(html);
    } else {
      htmlParts.push(html);
    }
  }

  flushSection();
  return htmlParts.join("");
}
