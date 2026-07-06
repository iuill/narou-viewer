import { afterEach, describe, expect, it } from "vitest";
import {
  startFakeNovelFetcherServer,
  startFakeOpenRouterServer,
  type RunningFakeServer
} from "./harness/fakeServers.js";

const runningServers: RunningFakeServer[] = [];

async function track<T extends RunningFakeServer>(server: T): Promise<T> {
  runningServers.push(server);
  return server;
}

afterEach(async () => {
  await Promise.all(runningServers.splice(0).map((server) => server.close()));
});

describe("backend fake servers", () => {
  it("records OpenRouter-compatible chat completion requests and returns deterministic content", async () => {
    const server = await track(
      await startFakeOpenRouterServer({
        responseContent: {
          processedUpToEpisodeIndex: "2",
          characters: []
        }
      })
    );

    const response = await fetch(`${server.baseUrl}/api/v1/chat/completions`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        authorization: "Bearer sk-test"
      },
      body: JSON.stringify({
        model: "openai/gpt-5-mini",
        messages: [{ role: "user", content: "hello" }]
      })
    });

    await expect(response.json()).resolves.toEqual(
      expect.objectContaining({
        id: "fake-openrouter-completion",
        choices: [
          {
            message: {
              content: JSON.stringify({
                processedUpToEpisodeIndex: "2",
                characters: []
              })
            }
          }
        ]
      })
    );
    expect(server.requests).toHaveLength(1);
    expect(server.requests[0]).toMatchObject({
      method: "POST",
      pathname: "/api/v1/chat/completions",
      json: expect.objectContaining({
        model: "openai/gpt-5-mini"
      })
    });
  });

  it("serves a deterministic novel-fetcher work, toc, and episode API", async () => {
    const server = await track(
      await startFakeNovelFetcherServer([
        {
          id: "1",
          siteWorkId: "n1234ab",
          title: "Fixture Work",
          author: "Fixture Author",
          episodes: [
            {
              episodeIndex: "1",
              title: "第一話",
              body: "本文"
            }
          ]
        }
      ])
    );

    await expect(fetch(`${server.baseUrl}/api/v1/works`).then((response) => response.json())).resolves.toMatchObject({
      works: [
        {
          id: "1",
          site_work_id: "n1234ab",
          source_url: "https://ncode.syosetu.com/n1234ab/",
          title: "Fixture Work",
          author: "Fixture Author",
          episode_count: 1,
          saved_episode_count: 1,
          expected_episode_count: 1
        }
      ]
    });
    await expect(fetch(`${server.baseUrl}/api/v1/works/1/toc`).then((response) => response.json())).resolves.toMatchObject({
      id: "1",
      site_work_id: "n1234ab",
      title: "Fixture Work",
      author: "Fixture Author",
      episodes: [
        {
          episode_id: "1",
          display_index: "1",
          source_url: "https://ncode.syosetu.com/n1234ab/1/",
          title: "第一話",
          body_status: "complete"
        }
      ]
    });
    await expect(fetch(`${server.baseUrl}/api/v1/works/1/episodes/1`).then((response) => response.json())).resolves.toMatchObject({
      work: {
        id: "1",
        site_work_id: "n1234ab",
        title: "Fixture Work"
      },
      episode: {
        episode_id: "1",
        display_index: "1",
        source_url: "https://ncode.syosetu.com/n1234ab/1/",
        title: "第一話"
      },
      canonical: {
        episode_id: "1",
        display_index: "1",
        source_url: "https://ncode.syosetu.com/n1234ab/1/",
        title: "第一話",
        blocks: [{ type: "paragraph", section: "body", text: "本文" }]
      }
    });
    await expect(fetch(`${server.baseUrl}/api/v1/works/work-1/toc`)).resolves.toMatchObject({
      status: 400
    });
    expect(server.requests.map((request) => request.pathname)).toEqual([
      "/api/v1/works",
      "/api/v1/works/1/toc",
      "/api/v1/works/1/episodes/1",
      "/api/v1/works/work-1/toc"
    ]);
  });
});
