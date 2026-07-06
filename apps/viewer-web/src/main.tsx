import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { registerServiceWorker } from "./registerServiceWorker";
import "./styles/index.css";

type BootstrapDependencies = {
  registerServiceWorker?: typeof registerServiceWorker;
  createRoot?: typeof ReactDOM.createRoot;
  rootElement?: Element | DocumentFragment;
  AppComponent?: typeof App;
};

export function bootstrap({
  registerServiceWorker: currentRegisterServiceWorker = registerServiceWorker,
  createRoot = ReactDOM.createRoot,
  rootElement,
  AppComponent = App
}: BootstrapDependencies = {}): void {
  currentRegisterServiceWorker();

  const mountElement = rootElement ?? document.getElementById("root");
  if (!mountElement) {
    throw new Error("Root element #root was not found.");
  }

  createRoot(mountElement).render(
    <React.StrictMode>
      <AppComponent />
    </React.StrictMode>
  );
}

if (typeof document !== "undefined") {
  bootstrap();
}
