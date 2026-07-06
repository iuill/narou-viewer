export type EpisodeIndex = string;

export type TocEpisode = {
  episodeIndex: EpisodeIndex;
  title: string;
  chapter: string | null;
  subchapter: string | null;
  sourceUrl: string | null;
  updatedAt: string | null;
  contentEtag: string;
  bodyStatus?: string;
  lastFetchError?: string | null;
};

export type TocResponse = {
  novelId: string;
  fetcherWorkId: string;
  title: string;
  author: string;
  siteName: string;
  tocUrl: string | null;
  updatedAt: string;
  lastActivityAt: string | null;
  totalEpisodes: number;
  story: string;
  episodes: TocEpisode[];
};

export type ReaderState = {
  novelId: string;
  lastReadEpisodeIndex: EpisodeIndex | null;
  position: number;
  updatedAt: string | null;
  stateVersion: number;
  updatedByClientId: string | null;
};

export type Bookmark = {
  id: string;
  novelId: string;
  episodeIndex: EpisodeIndex;
  position: number;
  label: string | null;
  createdAt: string;
};

export type BookmarksResponse = {
  bookmarks: Bookmark[];
};

export type ReadingMode = "vertical" | "horizontal";
export type ReaderFontFamily = "mincho" | "gothic";
export type ReaderTheme = "classic" | "paper" | "forest" | "ocean" | "midnight";

export type ReaderPreferencesResponse = {
  readingMode: ReadingMode;
  fontFamily: ReaderFontFamily;
  theme: ReaderTheme;
  updatedAt: string | null;
};

export type NovelReaderSettingsResponse = {
  novelId: string;
  correction: {
    quoteNormalization: boolean;
    hyphenDashNormalization: boolean;
    parenthesisNormalization: boolean;
    halfwidthAlnumPunctuationNormalization: boolean;
  };
  updatedAt: string | null;
};

export type NovelReaderCorrectionPatch = Partial<NovelReaderSettingsResponse["correction"]>;

export type ReaderAiAssistantHistoryMessage = {
  role: "user" | "assistant";
  text: string;
};

export type ReaderAiAssistantChatRequest = {
  message: string;
  currentEpisodeIndex: EpisodeIndex;
  position: number;
  history: ReaderAiAssistantHistoryMessage[];
};

export type ReaderAiAssistantToolResult = {
  name: string;
  result: unknown;
};

export type ReaderAiAssistantToolRequest = {
  name: string;
  arguments: unknown;
};

export type ReaderAiAssistantResponse = {
  answer: string;
  novelId: string;
  maxEpisodeIndex: string;
  runId: string | null;
  toolRequests: ReaderAiAssistantToolRequest[];
  toolResults: ReaderAiAssistantToolResult[];
  generationMode: "remote" | "local";
};

export type ReaderAiAssistantStreamEvent =
  | {
      type: "status";
      message: string;
    }
  | {
      type: "tool_call";
      toolName: string;
      message: string;
    }
  | {
      type: "tool_result";
      toolName: string;
      message: string;
    }
  | {
      type: "result";
      response: ReaderAiAssistantResponse;
    }
  | {
      type: "error";
      error: string;
    };

export type ReaderSectionRole = "introduction" | "body" | "postscript";

export type ReaderInlineToken =
  | { type: "text"; text: string }
  | { type: "ruby"; text: string; ruby: string }
  | { type: "lineBreak" }
  | { type: "tcy"; text: string }
  | { type: "link"; href: string | null; children: ReaderInlineToken[] };

export type ReaderBlock =
  | { type: "meta"; role: "chapter" | "subchapter"; text: string }
  | { type: "title"; text: string }
  | { type: "paragraph"; section: ReaderSectionRole; inlines: ReaderInlineToken[] }
  | {
      type: "image";
      section: ReaderSectionRole;
      src: string;
      alt: string | null;
      originalUrl: string | null;
      title: string | null;
      width?: number | null;
      height?: number | null;
    }
  | {
      type: "html";
      section: ReaderSectionRole;
      html: string;
      plainText: string;
    };

export type ReaderDocumentResponse = {
  version: 1;
  blocks: ReaderBlock[];
};

export type EpisodeResponse = {
  novelId: string;
  episodeIndex: EpisodeIndex;
  title: string;
  chapter: string | null;
  subchapter: string | null;
  sourceUrl?: string | null;
  html: string;
  readerDocument: ReaderDocumentResponse;
  plainTextLength: number;
  updatedAt: string;
  contentEtag: string;
};
