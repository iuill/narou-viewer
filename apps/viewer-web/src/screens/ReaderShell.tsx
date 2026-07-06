import {
  ReaderScreen,
  type ReaderScreenCommands,
  type ReaderScreenRefs,
  type ReaderScreenState
} from "../features/reader/ReaderScreen";

export type ReaderScreenModel = {
  commands: ReaderScreenCommands;
  refs: ReaderScreenRefs;
  state: ReaderScreenState;
};

export function ReaderShell({ commands, refs, state }: ReaderScreenModel) {
  return <ReaderScreen commands={commands} refs={refs} state={state} />;
}
