import { forwardRef, type ComponentProps } from "react";
import { AiGenerationWorkspace } from "./AiGenerationWorkspace";

export type AiGenerationWorkspaceProps = ComponentProps<typeof AiGenerationWorkspace>;

export type AiGenerationWorkspaceHostProps = Omit<AiGenerationWorkspaceProps, "activeView"> & {
  activeView: AiGenerationWorkspaceProps["activeView"] | null;
};

export const AiGenerationWorkspaceHost = forwardRef<HTMLDivElement, AiGenerationWorkspaceHostProps>(
  function AiGenerationWorkspaceHost({ activeView, ...props }, ref) {
    return activeView ? (
      <div className="ai-generation-workspace-host" ref={ref} tabIndex={-1}>
        <AiGenerationWorkspace {...props} activeView={activeView} />
      </div>
    ) : null;
  }
);
