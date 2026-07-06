type ServiceWorkerRegistrationDependencies = {
  isProd?: boolean;
  navigator?: Pick<Navigator, "serviceWorker"> | undefined;
  window?: Pick<Window, "addEventListener"> | undefined;
  console?: Pick<Console, "error">;
};

export function registerServiceWorker({
  isProd = import.meta.env.PROD,
  navigator: currentNavigator = globalThis.navigator,
  window: currentWindow = globalThis.window,
  console: currentConsole = console
}: ServiceWorkerRegistrationDependencies = {}) {
  if (!isProd) {
    return;
  }

  if (!currentNavigator || !currentWindow) {
    return;
  }

  if (!("serviceWorker" in currentNavigator)) {
    return;
  }

  currentWindow.addEventListener("load", () => {
    void currentNavigator.serviceWorker.register("/reader-sw.js").catch((error: unknown) => {
      currentConsole.error("Failed to register reader service worker", error);
    });
  });
}
