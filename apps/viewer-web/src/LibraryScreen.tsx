import type { ComponentProps, ReactNode, Ref } from "react";
import { AiGenerationWorkspaceHost } from "./AiGenerationWorkspaceHost";
import { LibraryPanel } from "./LibraryPanel";
import { TocPanel } from "./TocPanel";

type LibraryScreenProps = {
  activeMobileLibraryPanel: "library" | "details";
  aiGenerationWorkspaceRef: Ref<HTMLDivElement>;
  aiGenerationWorkspaceProps: ComponentProps<typeof AiGenerationWorkspaceHost>;
  isInitialLoading: boolean;
  isMobileLibraryViewport: boolean;
  libraryPanelProps: ComponentProps<typeof LibraryPanel>;
  mobileHomeTab: "library" | "download" | "status";
  mobileStatusPanel: ReactNode;
  onMobileHomeTabChange: (tab: "library" | "download" | "status") => void;
  tocPanelProps: ComponentProps<typeof TocPanel> | null;
};

export function LibraryScreen({
  activeMobileLibraryPanel,
  aiGenerationWorkspaceRef,
  aiGenerationWorkspaceProps,
  isInitialLoading,
  isMobileLibraryViewport,
  libraryPanelProps,
  mobileHomeTab,
  mobileStatusPanel,
  onMobileHomeTabChange,
  tocPanelProps
}: LibraryScreenProps): ReactNode {
  if (isInitialLoading) {
    return <p className="message">初期データを読み込み中...</p>;
  }

  return (
    <>
      <AiGenerationWorkspaceHost {...aiGenerationWorkspaceProps} ref={aiGenerationWorkspaceRef} />
      {isMobileLibraryViewport ? (
        <div className={`mobile-home mobile-home-tab-${mobileHomeTab}`}>
          {mobileHomeTab === "status" ? (
            mobileStatusPanel
          ) : (
            <div className={`workspace-grid mobile-view-${activeMobileLibraryPanel}`}>
              {activeMobileLibraryPanel === "details" && tocPanelProps ? (
                <TocPanel {...tocPanelProps} />
              ) : (
                <LibraryPanel {...libraryPanelProps} mobileHomeTab={mobileHomeTab} />
              )}
            </div>
          )}
          <nav aria-label="トップページ" className="mobile-home-tabs">
            <button
              aria-current={mobileHomeTab === "library" ? "page" : undefined}
              className={mobileHomeTab === "library" ? "active" : ""}
              onClick={() => onMobileHomeTabChange("library")}
              type="button"
            >
              <span aria-hidden="true">本</span>
              <strong>ライブラリ</strong>
            </button>
            <button
              aria-current={mobileHomeTab === "download" ? "page" : undefined}
              className={mobileHomeTab === "download" ? "active" : ""}
              onClick={() => onMobileHomeTabChange("download")}
              type="button"
            >
              <span aria-hidden="true">↓</span>
              <strong>取得</strong>
            </button>
            <button
              aria-current={mobileHomeTab === "status" ? "page" : undefined}
              className={mobileHomeTab === "status" ? "active" : ""}
              onClick={() => onMobileHomeTabChange("status")}
              type="button"
            >
              <span aria-hidden="true">●</span>
              <strong>状況</strong>
            </button>
          </nav>
        </div>
      ) : (
        <div className="workspace-grid">
          <LibraryPanel {...libraryPanelProps} />
          {tocPanelProps ? <TocPanel {...tocPanelProps} /> : null}
        </div>
      )}
    </>
  );
}
