import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@shared/styles.css";
import { I18nProvider } from "@shared/i18n";
import { ThemeProvider } from "@shared/theme";
import { ConfirmProvider, ToastProvider } from "@shared/overlay";
import { dicts } from "./i18n";
import { App } from "./App";

const qc = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: true, staleTime: 15_000 } },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={qc}>
      <I18nProvider dicts={dicts}>
        <ThemeProvider>
          <ToastProvider>
            <ConfirmProvider>
              <BrowserRouter>
                <App />
              </BrowserRouter>
            </ConfirmProvider>
          </ToastProvider>
        </ThemeProvider>
      </I18nProvider>
    </QueryClientProvider>
  </StrictMode>,
);
