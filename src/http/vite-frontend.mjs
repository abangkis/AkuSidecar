export async function attachViteFrontend(app, config) {
  const { createServer } = await import("vite");
  const vite = await createServer({
    root: config.publicDirectory,
    publicDir: false,
    appType: "spa",
    clearScreen: false,
    server: {
      middlewareMode: true,
      hmr: {
        server: app.server,
      },
    },
  });

  app.setFrontend({
    name: "vite",
    middleware: vite.middlewares,
    close: () => vite.close(),
  });

  return vite;
}
