import { useEffect, useState } from "react";
import {
  API_CLIENT_UPDATE_REQUIRED_EVENT,
  type ApiClientUpdateRequiredEventDetail
} from "../api/contract";
import {
  fetchLatestViewerBuildInfo,
  isViewerBuildOutdated,
  viewerBuildInfo
} from "../buildInfo";

export function useClientUpdateRequirement() {
  const [clientUpdateRequired, setClientUpdateRequired] = useState<ApiClientUpdateRequiredEventDetail | null>(null);

  useEffect(() => {
    const handleClientUpdateRequired = (event: Event) => {
      const detail =
        event instanceof CustomEvent && event.detail && typeof event.detail === "object"
          ? (event.detail as ApiClientUpdateRequiredEventDetail)
          : {
              message: "アプリの更新が必要です。",
              code: "CLIENT_UPDATE_REQUIRED",
              status: 426,
              minApiContractVersion: null
            };
      setClientUpdateRequired(detail);
    };

    window.addEventListener(API_CLIENT_UPDATE_REQUIRED_EVENT, handleClientUpdateRequired);
    return () => window.removeEventListener(API_CLIENT_UPDATE_REQUIRED_EVENT, handleClientUpdateRequired);
  }, []);

  useEffect(() => {
    let active = true;

    void fetchLatestViewerBuildInfo()
      .then((latestBuildInfo) => {
        if (!active || !isViewerBuildOutdated(viewerBuildInfo, latestBuildInfo)) {
          return;
        }

        setClientUpdateRequired({
          message: "バージョンアップしました。アプリを再読み込みしてください。",
          code: "CLIENT_UPDATE_REQUIRED",
          status: 426,
          minApiContractVersion: null
        });
      })
      .catch(() => {
        // 起動時チェックに失敗しても通常のAPIエラー検知で更新要求を拾えるため、画面表示は継続する。
      });

    return () => {
      active = false;
    };
  }, []);

  return { clientUpdateRequired, setClientUpdateRequired };
}
