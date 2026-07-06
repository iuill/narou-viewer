import http from "node:http";

export type RunningFakeServer = {
  baseUrl: string;
  close: () => Promise<void>;
};

type RecordedRequest = {
  method: string;
  pathname: string;
  headers: http.IncomingHttpHeaders;
  bodyText: string;
  json: unknown;
};

async function readBody(request: http.IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }
  return Buffer.concat(chunks).toString("utf8");
}

function writeJson(response: http.ServerResponse, status: number, payload: unknown): void {
  response.writeHead(status, {
    "content-type": "application/json"
  });
  response.end(JSON.stringify(payload));
}

function parseJsonOrNull(bodyText: string): unknown {
  if (bodyText.trim().length === 0) {
    return null;
  }

  try {
    return JSON.parse(bodyText) as unknown;
  } catch {
    return null;
  }
}

async function listen(server: http.Server): Promise<RunningFakeServer> {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("Failed to start fake server.");
  }

  return {
    baseUrl: `http://127.0.0.1:${address.port}`,
    close: () => new Promise((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())))
  };
}

export async function startFakeOpenRouterServer(options: {
  responseContent?: unknown;
  failFirstRequestWith?: number;
} = {}): Promise<RunningFakeServer & { requests: RecordedRequest[] }> {
  const requests: RecordedRequest[] = [];
  let requestCount = 0;
  const server = http.createServer(async (request, response) => {
    const url = new URL(request.url ?? "/", "http://127.0.0.1");
    const bodyText = await readBody(request);
    const recorded = {
      method: request.method ?? "GET",
      pathname: url.pathname,
      headers: request.headers,
      bodyText,
      json: parseJsonOrNull(bodyText)
    } satisfies RecordedRequest;
    requests.push(recorded);
    requestCount += 1;

    if (url.pathname !== "/api/v1/chat/completions" || request.method !== "POST") {
      writeJson(response, 404, { error: { message: "not found" } });
      return;
    }

    if (options.failFirstRequestWith && requestCount === 1) {
      writeJson(response, options.failFirstRequestWith, { error: { message: "temporary fake failure" } });
      return;
    }

    writeJson(response, 200, {
      id: "fake-openrouter-completion",
      choices: [
        {
          message: {
            content: JSON.stringify(
              options.responseContent ?? {
                processedUpToEpisodeIndex: "1",
                characters: []
              }
            )
          }
        }
      ]
    });
  });

  return {
    ...(await listen(server)),
    requests
  };
}

export type FakeNovelFetcherWork = {
  id: string;
  siteWorkId?: string;
  title: string;
  author: string;
  episodes: Array<{
    episodeIndex: string;
    title: string;
    body: string;
  }>;
};

function fakeSiteWorkId(work: FakeNovelFetcherWork): string {
  return work.siteWorkId ?? work.id;
}

function isPositiveIntegerId(value: string): boolean {
  return /^[1-9]\d*$/.test(value);
}

function fakeWorkPayload(work: FakeNovelFetcherWork): Record<string, unknown> {
  const siteWorkId = fakeSiteWorkId(work);
  return {
    id: work.id,
    site: "narou",
    site_name: "小説家になろう",
    site_work_id: siteWorkId,
    source_url: `https://ncode.syosetu.com/${siteWorkId}/`,
    title: work.title,
    author: work.author,
    story: "",
    directory: work.id,
    fetched_at: "2026-01-01T00:00:00Z",
    episode_count: work.episodes.length,
    saved_episode_count: work.episodes.length,
    fetch_status: "complete",
    last_fetch_error: null,
    failed_episode_id: null,
    resume_episode_id: null,
    expected_episode_count: work.episodes.length
  };
}

function fakeEpisodeSummaryPayload(
  work: FakeNovelFetcherWork,
  episode: FakeNovelFetcherWork["episodes"][number]
): Record<string, unknown> {
  const siteWorkId = fakeSiteWorkId(work);
  return {
    episode_id: episode.episodeIndex,
    site_episode_id: episode.episodeIndex,
    source_url: `https://ncode.syosetu.com/${siteWorkId}/${episode.episodeIndex}/`,
    sort_order: Number.parseInt(episode.episodeIndex, 10) || 0,
    display_index: episode.episodeIndex,
    title: episode.title,
    chapter: "",
    subchapter: "",
    published_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    content_hash: `fake-${episode.episodeIndex}`,
    fetched_at: "2026-01-01T00:00:00Z",
    body_status: "complete",
    last_fetch_error: null
  };
}

export async function startFakeNovelFetcherServer(
  works: FakeNovelFetcherWork[]
): Promise<RunningFakeServer & { requests: RecordedRequest[] }> {
  const invalidWork = works.find((work) => !isPositiveIntegerId(work.id));
  if (invalidWork) {
    throw new Error(`Fake novel-fetcher work id must be a positive integer: ${invalidWork.id}`);
  }

  const requests: RecordedRequest[] = [];
  const server = http.createServer(async (request, response) => {
    const url = new URL(request.url ?? "/", "http://127.0.0.1");
    const bodyText = await readBody(request);
    requests.push({
      method: request.method ?? "GET",
      pathname: url.pathname,
      headers: request.headers,
      bodyText,
      json: parseJsonOrNull(bodyText)
    });

    if (url.pathname === "/api/v1/works" && request.method === "GET") {
      writeJson(response, 200, { works: works.map(fakeWorkPayload) });
      return;
    }

    const tocMatch = url.pathname.match(/^\/api\/v1\/works\/([^/]+)\/toc$/);
    if (tocMatch && request.method === "GET") {
      const workId = decodeURIComponent(tocMatch[1] ?? "");
      if (!isPositiveIntegerId(workId)) {
        writeJson(response, 400, { error: "work id must be a positive integer" });
        return;
      }
      const work = works.find((candidate) => candidate.id === workId);
      if (!work) {
        writeJson(response, 404, { error: "work not found" });
        return;
      }
      writeJson(response, 200, {
        ...fakeWorkPayload(work),
        episodes: work.episodes.map((episode) => fakeEpisodeSummaryPayload(work, episode))
      });
      return;
    }

    const episodeMatch = url.pathname.match(/^\/api\/v1\/works\/([^/]+)\/episodes\/([^/]+)$/);
    if (episodeMatch && request.method === "GET") {
      const workId = decodeURIComponent(episodeMatch[1] ?? "");
      if (!isPositiveIntegerId(workId)) {
        writeJson(response, 400, { error: "work id must be a positive integer" });
        return;
      }
      const work = works.find((candidate) => candidate.id === workId);
      const episode = work?.episodes.find((candidate) => candidate.episodeIndex === decodeURIComponent(episodeMatch[2] ?? ""));
      if (!work || !episode) {
        writeJson(response, 404, { error: "episode not found" });
        return;
      }
      writeJson(response, 200, {
        work: fakeWorkPayload(work),
        episode: fakeEpisodeSummaryPayload(work, episode),
        canonical: {
          episode_id: episode.episodeIndex,
          site_episode_id: episode.episodeIndex,
          source_url: `https://ncode.syosetu.com/${fakeSiteWorkId(work)}/${episode.episodeIndex}/`,
          display_index: episode.episodeIndex,
          title: episode.title,
          chapter: "",
          subchapter: "",
          updated_at: "2026-01-01T00:00:00Z",
          fetched_at: "2026-01-01T00:00:00Z",
          blocks: [{ type: "paragraph", section: "body", text: episode.body }]
        }
      });
      return;
    }

    writeJson(response, 404, { error: "not found" });
  });

  return {
    ...(await listen(server)),
    requests
  };
}
