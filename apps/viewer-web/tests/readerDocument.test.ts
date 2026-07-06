import { afterEach, describe, expect, it, vi } from "vitest";
import { JSDOM } from "jsdom";
import { renderReaderDocumentHtml, type ReaderDocument } from "../src/readerDocument";

describe("readerDocument", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function installDom(): void {
    const dom = new JSDOM("<!doctype html><html><body></body></html>");
    vi.stubGlobal("document", dom.window.document);
  }

  it("renders readerDocument images without anchor wrappers and preserves original metadata", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "image",
          section: "body",
          src: "/images/example.png",
          alt: "挿絵",
          originalUrl: "https://example.com/original.png",
          title: "元画像"
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument)).toBe(
      '<section class="reader-section reader-section-body"><p><img src="/images/example.png" alt="挿絵" title="元画像" data-reader-image-original-href="https://example.com/original.png" data-reader-image-original-title="元画像" data-reader-pagination-fragment="image" data-reader-position-start="0" data-reader-position-end="1"></p></section>'
    );
  });

  it("drops unsafe href values from rendered inline links", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [
            {
              type: "link",
              href: "javascript:alert(1)",
              children: [{ type: "text", text: "危険リンク" }]
            }
          ]
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument, { enableVisibilityFragments: true })).toBe(
      '<section class="reader-section reader-section-body"><p><a><span data-reader-pagination-fragment="text" data-reader-position-start="0" data-reader-position-end="5"><span data-reader-visibility-fragment="text">危</span><span data-reader-visibility-fragment="text">険</span><span data-reader-visibility-fragment="text">リ</span><span data-reader-visibility-fragment="text">ン</span><span data-reader-visibility-fragment="text">ク</span></span></a></p></section>'
    );
  });

  it("drops image blocks with unsafe src values", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "image",
          section: "body",
          src: "javascript:alert(1)",
          alt: "危険画像",
          originalUrl: "https://example.com/original.png",
          title: "元画像"
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument)).toBe("");
  });

  it("wraps measurable text and fallback html fragments for vertical pagination", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "title",
          text: "表題"
        },
        {
          type: "paragraph",
          section: "body",
          inlines: [
            { type: "text", text: "本文" },
            { type: "tcy", text: "2026" },
            { type: "ruby", text: "続", ruby: "つづ" }
          ]
        },
        {
          type: "html",
          section: "postscript",
          html: "<blockquote>補足</blockquote>",
          plainText: "補足"
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument, { enableVisibilityFragments: true })).toBe(
      '<h1 class="reader-title"><span data-reader-pagination-fragment="title" data-reader-position-start="0" data-reader-position-end="2"><span data-reader-visibility-fragment="title">表</span><span data-reader-visibility-fragment="title">題</span></span></h1><section class="reader-section reader-section-body"><p><span data-reader-pagination-fragment="text" data-reader-position-start="2" data-reader-position-end="4"><span data-reader-visibility-fragment="text">本</span><span data-reader-visibility-fragment="text">文</span></span><span data-reader-pagination-fragment="tcy" class="reader-tcy" data-reader-position-start="4" data-reader-position-end="8"><span data-reader-visibility-fragment="tcy">2</span><span data-reader-visibility-fragment="tcy">0</span><span data-reader-visibility-fragment="tcy">2</span><span data-reader-visibility-fragment="tcy">6</span></span><ruby data-reader-pagination-fragment="ruby" data-reader-position-start="8" data-reader-position-end="9"><span data-reader-visibility-fragment="ruby">続</span><rt>つづ</rt></ruby></p></section><section class="reader-section reader-section-postscript"><div class="reader-html-fragment" data-reader-pagination-fragment="html" data-reader-position-start="9" data-reader-position-end="10"><blockquote>補足</blockquote></div></section>'
    );
  });

  it("wraps repeated horizontal bars without changing reader positions", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: "錬金術――――2つ" }]
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument)).toBe(
      '<section class="reader-section reader-section-body"><p><span data-reader-pagination-fragment="text" data-reader-position-start="0" data-reader-position-end="9">錬金術<span class="reader-dash-run">――――</span>2つ</span></p></section>'
    );
  });

  it("keeps visibility fragments inside repeated horizontal bar runs", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: "A――B" }]
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument, { enableVisibilityFragments: true })).toBe(
      '<section class="reader-section reader-section-body"><p><span data-reader-pagination-fragment="text" data-reader-position-start="0" data-reader-position-end="4"><span data-reader-visibility-fragment="text">A</span><span class="reader-dash-run"><span data-reader-visibility-fragment="text">―</span><span data-reader-visibility-fragment="text">―</span></span><span data-reader-visibility-fragment="text">B</span></span></p></section>'
    );
  });

  it("wraps repeated horizontal bars in fallback html text nodes", () => {
    installDom();
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "html",
          section: "body",
          html: "<blockquote>錬金術――――2つ</blockquote><ul><li>相方――――ゴーレム</li></ul><p>段落――――終わり</p>",
          plainText: "錬金術――――2つ 相方――――ゴーレム 段落――――終わり"
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument)).toBe(
      '<section class="reader-section reader-section-body"><div class="reader-html-fragment" data-reader-pagination-fragment="html" data-reader-position-start="0" data-reader-position-end="1"><blockquote>錬金術<span class="reader-dash-run">――――</span>2つ</blockquote><ul><li>相方<span class="reader-dash-run">――――</span>ゴーレム</li></ul><p>段落<span class="reader-dash-run">――――</span>終わり</p></div></section>'
    );
  });

  it("wraps fallback html horizontal bars across inline node boundaries", () => {
    installDom();
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "html",
          section: "body",
          html: '<p><a href="/">―</a><em>―</em><strong>―</strong><span>―</span><ruby>―<rt>よみ</rt></ruby>終わり</p>',
          plainText: "―――――よみ終わり"
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument)).toBe(
      '<section class="reader-section reader-section-body"><div class="reader-html-fragment" data-reader-pagination-fragment="html" data-reader-position-start="0" data-reader-position-end="1"><p><a href="/"><span class="reader-dash-run reader-dash-run-split-start">―</span></a><em><span class="reader-dash-run reader-dash-run-split-middle">―</span></em><strong><span class="reader-dash-run reader-dash-run-split-middle">―</span></strong><span><span class="reader-dash-run reader-dash-run-split-middle">―</span></span><ruby><span class="reader-dash-run reader-dash-run-split-end">―</span><rt>よみ</rt></ruby>終わり</p></div></section>'
    );
  });

  it("wraps local fallback html horizontal bars in text nodes that also join cross-node bars", () => {
    installDom();
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "html",
          section: "body",
          html: "<p><span>―</span><em>― X ――</em></p>",
          plainText: "―― X ――"
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument)).toBe(
      '<section class="reader-section reader-section-body"><div class="reader-html-fragment" data-reader-pagination-fragment="html" data-reader-position-start="0" data-reader-position-end="1"><p><span><span class="reader-dash-run reader-dash-run-split-start">―</span></span><em><span class="reader-dash-run reader-dash-run-split-end">―</span> X <span class="reader-dash-run">――</span></em></p></div></section>'
    );
  });

  it("does not wrap fallback html horizontal bars across block boundaries", () => {
    installDom();
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "html",
          section: "body",
          html: "<p>―</p><p>―</p><ul><li>―</li><li>―</li></ul><blockquote>―</blockquote><p>―</p>",
          plainText: "― ― ― ― ― ―"
        }
      ]
    };

    expect(renderReaderDocumentHtml(readerDocument)).toBe(
      '<section class="reader-section reader-section-body"><div class="reader-html-fragment" data-reader-pagination-fragment="html" data-reader-position-start="0" data-reader-position-end="1"><p>―</p><p>―</p><ul><li>―</li><li>―</li></ul><blockquote>―</blockquote><p>―</p></div></section>'
    );
  });

  it("splits long repeated horizontal bars into breakable dash runs", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: `区切り${"―".repeat(30)}終わり` }]
        }
      ]
    };

    const html = renderReaderDocumentHtml(readerDocument);
    expect(html).toContain(
      `区切り<span class="reader-dash-run reader-dash-run-split-start">${"―".repeat(12)}</span><span class="reader-dash-run reader-dash-run-split-middle">${"―".repeat(12)}</span><span class="reader-dash-run reader-dash-run-split-end">${"―".repeat(6)}</span>終わり`
    );
  });
});
