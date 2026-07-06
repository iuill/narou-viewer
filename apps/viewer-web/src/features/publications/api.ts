import { requestJson } from "../../api/http";
import type {
  NovelPublicationsResponse,
  PublicationDisplayCoverRequest,
  PublicationEntryRequest
} from "./types";

export function fetchNovelPublications(novelId: string): Promise<NovelPublicationsResponse> {
  return requestJson<NovelPublicationsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/publications`,
    undefined,
    "書籍情報の取得に失敗しました。"
  );
}

export function createPublicationEntry(
  novelId: string,
  body: PublicationEntryRequest
): Promise<NovelPublicationsResponse> {
  return requestJson<NovelPublicationsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/publications/entries`,
    {
      method: "POST",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify(body)
    },
    "書籍情報の保存に失敗しました。"
  );
}

export function putPublicationEntry(
  novelId: string,
  entryId: string,
  body: PublicationEntryRequest
): Promise<NovelPublicationsResponse> {
  return requestJson<NovelPublicationsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/publications/entries/${encodeURIComponent(entryId)}`,
    {
      method: "PUT",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify(body)
    },
    "書籍情報の保存に失敗しました。"
  );
}

export function putPublicationDisplayCover(
  novelId: string,
  body: PublicationDisplayCoverRequest
): Promise<NovelPublicationsResponse> {
  return requestJson<NovelPublicationsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/publications/display-cover`,
    {
      method: "PUT",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify(body)
    },
    "一覧表紙の保存に失敗しました。"
  );
}
