import { useViewerAppModel } from "./app/useViewerAppModel";
import { ReaderRouteController } from "./routes/ReaderRouteController";
import { LibraryShell } from "./screens/LibraryShell";
import { ReaderShell } from "./screens/ReaderShell";

export default function App() {
  const app = useViewerAppModel();

  return (
    <>
      <ReaderRouteController {...app.routeController} />
      {app.screen.type === "reader" ? (
        <ReaderShell {...app.screen.reader} />
      ) : (
        <LibraryShell {...app.screen.library} />
      )}
    </>
  );
}
